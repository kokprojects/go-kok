// Copyright 2014 The go-kokereum Authors
// This file is part of the go-kokereum library.
//
// The go-kokereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-kokereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-kokereum library. If not, see <http://www.gnu.org/licenses/>.

package core

import (
	"errors"
	"github.com/kokprojects/go-kok/core/types"
	"math/big"

	"github.com/kokprojects/go-kok/common"
	"github.com/kokprojects/go-kok/common/math"
	"github.com/kokprojects/go-kok/core/vm"
	"github.com/kokprojects/go-kok/log"
	"github.com/kokprojects/go-kok/params"
)

var (
	Big0                         = big.NewInt(0)
	errInsufficientBalanceForGas = errors.New("insufficient balance to pay for gas")
)

/*
The State Transitioning Model

A state transition is a change made when a transaction is applied to the current world state
The state transitioning model does all all the necessary work to work out a valid new state root.

1) Nonce handling
2) Pre pay gas
3) Create a new state object if the recipient is \0*32
4) Value transfer
== If contract creation ==
  4a) Attempt to run transaction data
  4b) If valid, use result as code for the new state object
== end ==
5) Run Script section
6) Derive new state root
*/
type StateTransition struct {
	gp         *GasPool
	msg        Message
	gas        uint64
	gasPrice   *big.Int
	initialGas *big.Int
	value      *big.Int
	data       []byte
	state      vm.StateDB
	evm        *vm.EVM
	ValidatorS []common.Address
	txType     types.TxType
	txHash     []byte
}

// Message represents a message sent to a contract.
type Message interface {
	From() common.Address
	//FromFrontier() (common.Address, error)
	To() *common.Address

	GasPrice() *big.Int
	Gas() *big.Int
	Value() *big.Int

	Nonce() uint64
	CheckNonce() bool
	Data() []byte
}

// IntrinsicGas computes the 'intrinsic gas' for a message
// with the given data.
//
// TODO convert to uint64
func IntrinsicGas(data []byte, contractCreation, homestead bool) *big.Int {
	igas := new(big.Int)
	if contractCreation && homestead {
		igas.SetUint64(params.TxGasContractCreation)
	} else {
		igas.SetUint64(params.TxGas)
	}
	if len(data) > 0 {
		var nz int64
		for _, byt := range data {
			if byt != 0 {
				nz++
			}
		}
		m := big.NewInt(nz)
		m.Mul(m, new(big.Int).SetUint64(params.TxDataNonZeroGas))
		igas.Add(igas, m)
		m.SetInt64(int64(len(data)) - nz)
		m.Mul(m, new(big.Int).SetUint64(params.TxDataZeroGas))
		igas.Add(igas, m)
	}
	return igas
}

// NewStateTransition initialises and returns a new state transition object.
func NewStateTransition(evm *vm.EVM, msg Message, gp *GasPool, Validators []common.Address, tx []byte, txtype types.TxType) *StateTransition {
	return &StateTransition{
		gp:         gp,
		evm:        evm,
		msg:        msg,
		gasPrice:   msg.GasPrice(),
		initialGas: new(big.Int),
		value:      msg.Value(),
		data:       msg.Data(),
		state:      evm.StateDB,
		ValidatorS: Validators,
		txType:     txtype,
		txHash:     tx,
	}
}

// ApplyMessage computes the new state by applying the given message
// against the old state within the environment.
//
// ApplyMessage returns the bytes returned by any EVM execution (if it took place),
// the gas used (which includes gas refunds) and an error if it failed. An error always
// indicates a core error meaning that the message would always fail for that particular
// state and would never be accepted within a block.
func ApplyMessage(evm *vm.EVM, msg Message, gp *GasPool, Validators []common.Address, txhash []byte, txtype types.TxType) ([]byte, *big.Int, bool, error) {
	st := NewStateTransition(evm, msg, gp, Validators, txhash, txtype)

	ret, _, gasUsed, failed, err := st.TransitionDb()
	return ret, gasUsed, failed, err
}

func (st *StateTransition) from() vm.AccountRef {
	f := st.msg.From()
	if !st.state.Exist(f) {
		st.state.CreateAccount(f)
	}
	return vm.AccountRef(f)
}

func (st *StateTransition) to() vm.AccountRef {
	if st.msg == nil {
		return vm.AccountRef{}
	}
	to := st.msg.To()
	if to == nil {
		return vm.AccountRef{} // contract creation
	}

	reference := vm.AccountRef(*to)
	if !st.state.Exist(*to) {
		st.state.CreateAccount(*to)
	}
	return reference
}

func (st *StateTransition) useGas(amount uint64) error {
	if st.gas < amount {
		return vm.ErrOutOfGas
	}
	st.gas -= amount

	return nil
}

func (st *StateTransition) buyGas() error {
	mgas := st.msg.Gas()
	if mgas.BitLen() > 64 {
		return vm.ErrOutOfGas
	}

	mgval := new(big.Int).Mul(mgas, st.gasPrice)

	var (
		state  = st.state
		sender = st.from()
	)
	if state.GetBalance(sender.Address()).Cmp(mgval) < 0 {
		return errInsufficientBalanceForGas
	}
	if err := st.gp.SubGas(mgas); err != nil {
		return err
	}
	st.gas += mgas.Uint64()

	st.initialGas.Set(mgas)
	state.SubBalance(sender.Address(), mgval)
	return nil
}

func (st *StateTransition) preCheck() error {
	msg := st.msg
	sender := st.from()

	// Make sure this transaction's nonce is correct
	if msg.CheckNonce() {
		nonce := st.state.GetNonce(sender.Address())
		if nonce < msg.Nonce() {
			return ErrNonceTooHigh
		} else if nonce > msg.Nonce() {
			return ErrNonceTooLow
		}
	}
	return st.buyGas()
}

// TransitionDb will transition the state by applying the current message and returning the result
// including the required gas for the operation as well as the used gas. It returns an error if it
// failed. An error indicates a consensus issue.
func (st *StateTransition) TransitionDb() (ret []byte, requiredGas, usedGas *big.Int, failed bool, err error) {
	if err = st.preCheck(); err != nil {
		return
	}
	msg := st.msg
	sender := st.from() // err checked in preCheck

	homestead := st.evm.ChainConfig().IsHomestead(st.evm.BlockNumber)

	var addressType string
	var contractCreation bool
	if msg.To() == nil {
		contractCreation = true
	} else {
		addressType = GetAddressType(st.state.GetState(*st.msg.To(), HashTypeString("type")))
		if addressType == "template" {
			contractCreation = true
		} else {
			contractCreation = false
		}
	}

	// Pay intrinsic gas
	// TODO convert to uint64
	intrinsicGas := IntrinsicGas(st.data, contractCreation, homestead)
	if intrinsicGas.BitLen() > 64 {
		return nil, nil, nil, false, vm.ErrOutOfGas
	}
	if err = st.useGas(intrinsicGas.Uint64()); err != nil {
		return nil, nil, nil, false, err
	}

	var (
		evm = st.evm
		// vm errors do not effect consensus and are therefor
		// not assigned to err, except for insufficient balance
		// error.
		vmerr error
	)
	if msg.To() == nil {
		//ret, _, st.gas, vmerr = evm.Create(sender, st.data, st.gas, st.value)
		ret, _, st.gas, vmerr = evm.Template(sender, st.data, st.gas, st.value)
	} else {
		// Increment the nonce for the next transaction
		switch addressType {
		case "template":

			if st.txType == types.Binary {
				templateCode := st.state.GetCode(*st.msg.To())
				if len(st.data) > 0 {
					templateCode = ByteAndByte(templateCode, st.data)
				}
				coinbase := st.state.GetState(*st.msg.To(), HashTypeString("coinbase"))
				templateCode = byteAppend(templateCode, coinbase)
				templateCode = byteAppendAddress(templateCode, *msg.To())
				ret, _, st.gas, vmerr = evm.Create(sender, templateCode, st.gas, st.value)
			} else if st.txType == types.SourceCode {
				st.state.SetNonce(msg.From(), st.state.GetNonce(sender.Address())+1)
				ret, st.gas, vmerr = evm.SourceCode(sender, *st.msg.To(), st.data, st.gas, st.value, nil, common.BytesToHash(st.txHash))
			} else if st.txType == types.Endorse {
				st.state.SetNonce(msg.From(), st.state.GetNonce(sender.Address())+1)
				ret, st.gas, vmerr = evm.Endorse(sender, *st.msg.To(), st.data, st.gas, st.value, nil, common.BytesToHash(st.txHash))
			} else {
				return nil, nil, nil, false, types.ErrInvalidType
			}
		case "contract":
			if st.txType == types.Endorse {
				st.state.SetNonce(msg.From(), st.state.GetNonce(sender.Address())+1)
				ret, st.gas, vmerr = evm.Endorse(sender, *st.msg.To(), st.data, st.gas, st.value, nil, common.BytesToHash(st.txHash))
			} else if st.txType == types.Binary {
				st.state.SetNonce(msg.From(), st.state.GetNonce(sender.Address())+1)
				ret, st.gas, vmerr = evm.Call(sender, *st.msg.To(), st.data, st.gas, st.value, nil)
			} else {
				return nil, nil, nil, false, types.ErrInvalidType
			}
		case "normal":
			if st.txType == types.Endorse {
				st.state.SetNonce(msg.From(), st.state.GetNonce(sender.Address())+1)
				ret, st.gas, vmerr = evm.Endorse(sender, *st.msg.To(), st.data, st.gas, st.value, nil, common.BytesToHash(st.txHash))
			} else if st.txType == types.Binary {
				st.state.SetNonce(msg.From(), st.state.GetNonce(sender.Address())+1)
				ret, st.gas, vmerr = evm.Call(sender, *st.msg.To(), st.data, st.gas, st.value, st.ValidatorS)
			} else if st.txType == types.LoginCandidate || st.txType == types.LogoutCandidate || st.txType == types.Delegate || st.txType == types.UnDelegate {

				if st.data != nil {
					return nil, nil, nil, false, types.ErrInvalidInput
				}
				st.state.SetNonce(msg.From(), st.state.GetNonce(sender.Address())+1)
				ret, st.gas, vmerr = evm.Call(sender, *st.msg.To(), st.data, st.gas, st.value, st.ValidatorS)

			} else {
				return nil, nil, nil, false, types.ErrInvalidType
			}
		}
	}
	if vmerr != nil {
		log.Debug("VM returned with error", "err", vmerr)
		// The only possible consensus-error would be if there wasn't
		// sufficient balance to make the transfer happen. The first
		// balance transfer may never fail.
		if vmerr == vm.ErrInsufficientBalance {
			return nil, nil, nil, false, vmerr
		}
	}
	requiredGas = new(big.Int).Set(st.gasUsed())

	st.refundGas()
	if addressType == "contract" {
		gas_mine, gas_coinbase := Layer(st.gasUsed().Uint64(), uint64(1))
		st.state.AddBalance(st.evm.Coinbase, new(big.Int).Mul(new(big.Int).SetUint64(gas_mine), st.gasPrice))
		address_coinbase := CommonHash2Address(st.state.GetState(*st.msg.To(), HashTypeString("coinbase")))
		st.state.AddBalance(address_coinbase, new(big.Int).Mul(new(big.Int).SetUint64(gas_coinbase), st.gasPrice))
	} else {
		st.state.AddBalance(st.evm.Coinbase, new(big.Int).Mul(new(big.Int).SetUint64(st.gasUsed().Uint64()), st.gasPrice))
	}
	return ret, requiredGas, st.gasUsed(), vmerr != nil, err
}

func Layer(gas, tail uint64) (gas_mine, gas_coinbase uint64) {
	gas_coinbase = gas / 10 * tail
	gas_mine = gas - gas_coinbase
	return
}

func CommonHash2Address(hash common.Hash) common.Address {
	address := common.Address{}
	for i := 0; i < len(address); i++ {
		address[i] = byte(hash[i])
	}
	return address
}

func (st *StateTransition) refundGas() {
	// Return kok for remaining gas to the sender account,
	// exchanged at the original rate.
	sender := st.from() // err already checked
	remaining := new(big.Int).Mul(new(big.Int).SetUint64(st.gas), st.gasPrice)
	st.state.AddBalance(sender.Address(), remaining)

	// Apply refund counter, capped to half of the used gas.
	uhalf := remaining.Div(st.gasUsed(), common.Big2)
	refund := math.BigMin(uhalf, st.state.GetRefund())
	st.gas += refund.Uint64()

	st.state.AddBalance(sender.Address(), refund.Mul(refund, st.gasPrice))

	// Also return remaining gas to the block gas counter so it is
	// available for the next transaction.
	st.gp.AddGas(new(big.Int).SetUint64(st.gas))
}

func (st *StateTransition) gasUsed() *big.Int {
	return new(big.Int).Sub(st.initialGas, new(big.Int).SetUint64(st.gas))
}

func HashTypeString(s string) common.Hash {
	hash := common.Hash{}
	code := []byte(s)
	for i := 0; i < len(s); i++ {
		hash[i] = code[i]
	}
	return hash
}

func GetAddressType(hash common.Hash) string {
	var st string
	for i := 0; i < 8; i++ {
		code := hash[i]
		code = byte(code)
		st += string(code)
	}

	switch st {
	case "template":
		return "template"
	case "contract":
		return "contract"
	default:
		return "normal"
	}

}

func byteAppend(template []byte, coinbase common.Hash) []byte {
	for i := 0; i < 20; i++ {
		template = append(template, byte(coinbase[i]))
	}
	return template
}

func byteAppendAddress(template []byte, address common.Address) []byte {
	for i := 0; i < 20; i++ {
		template = append(template, byte(address[i]))
	}
	return template
}

func ByteAndByte(template []byte, data []byte) []byte {

	for i := 0; i < len(data); i++ {
		template = append(template, byte(data[i]))
	}

	return template
}
