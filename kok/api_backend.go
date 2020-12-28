// Copyright 2015 The go-kokereum Authors
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

package kok

import (
	"context"
	"math/big"

	"github.com/kokprojects/go-kok/accounts"
	"github.com/kokprojects/go-kok/common"
	"github.com/kokprojects/go-kok/common/math"
	"github.com/kokprojects/go-kok/core"
	"github.com/kokprojects/go-kok/core/bloombits"
	"github.com/kokprojects/go-kok/core/state"
	"github.com/kokprojects/go-kok/core/types"
	"github.com/kokprojects/go-kok/core/vm"
	"github.com/kokprojects/go-kok/kok/downloader"
	"github.com/kokprojects/go-kok/kok/gasprice"
	"github.com/kokprojects/go-kok/kokdb"
	"github.com/kokprojects/go-kok/event"
	"github.com/kokprojects/go-kok/params"
	"github.com/kokprojects/go-kok/rpc"
)

// kokApiBackend implements kokapi.Backend for full nodes
type kokApiBackend struct {
	kok *kokereum
	gpo *gasprice.Oracle
}

func (b *kokApiBackend) ChainConfig() *params.ChainConfig {
	return b.kok.chainConfig
}

func (b *kokApiBackend) CurrentBlock() *types.Block {
	return b.kok.blockchain.CurrentBlock()
}

func (b *kokApiBackend) Skokead(number uint64) {
	b.kok.protocolManager.downloader.Cancel()
	b.kok.blockchain.Skokead(number)
}

func (b *kokApiBackend) HeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*types.Header, error) {
	// Pending block is only known by the miner
	if blockNr == rpc.PendingBlockNumber {
		block := b.kok.miner.PendingBlock()
		return block.Header(), nil
	}
	// Otherwise resolve and return the block
	if blockNr == rpc.LatestBlockNumber {
		return b.kok.blockchain.CurrentBlock().Header(), nil
	}
	return b.kok.blockchain.GkokeaderByNumber(uint64(blockNr)), nil
}

func (b *kokApiBackend) BlockByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*types.Block, error) {
	// Pending block is only known by the miner
	if blockNr == rpc.PendingBlockNumber {
		block := b.kok.miner.PendingBlock()
		return block, nil
	}
	// Otherwise resolve and return the block
	if blockNr == rpc.LatestBlockNumber {
		return b.kok.blockchain.CurrentBlock(), nil
	}
	return b.kok.blockchain.GetBlockByNumber(uint64(blockNr)), nil
}

func (b *kokApiBackend) StateAndHeaderByNumber(ctx context.Context, blockNr rpc.BlockNumber) (*state.StateDB, *types.Header, error) {
	// Pending state is only known by the miner
	if blockNr == rpc.PendingBlockNumber {
		block, state := b.kok.miner.Pending()
		return state, block.Header(), nil
	}
	// Otherwise resolve the block number and return its state
	header, err := b.HeaderByNumber(ctx, blockNr)
	if header == nil || err != nil {
		return nil, nil, err
	}
	stateDb, err := b.kok.BlockChain().StateAt(header.Root)
	return stateDb, header, err
}

func (b *kokApiBackend) GetBlock(ctx context.Context, blockHash common.Hash) (*types.Block, error) {
	return b.kok.blockchain.GetBlockByHash(blockHash), nil
}

func (b *kokApiBackend) GetReceipts(ctx context.Context, blockHash common.Hash) (types.Receipts, error) {
	return core.GetBlockReceipts(b.kok.chainDb, blockHash, core.GetBlockNumber(b.kok.chainDb, blockHash)), nil
}

func (b *kokApiBackend) GetTd(blockHash common.Hash) *big.Int {
	return b.kok.blockchain.GetTdByHash(blockHash)
}

func (b *kokApiBackend) GetEVM(ctx context.Context, msg core.Message, state *state.StateDB, header *types.Header, vmCfg vm.Config) (*vm.EVM, func() error, error) {
	state.SetBalance(msg.From(), math.MaxBig256)
	vmError := func() error { return nil }

	context := core.NewEVMContext(msg, header, b.kok.BlockChain(), nil)
	return vm.NewEVM(context, state, b.kok.chainConfig, vmCfg), vmError, nil
}

func (b *kokApiBackend) SubscribeRemovedLogsEvent(ch chan<- core.RemovedLogsEvent) event.Subscription {
	return b.kok.BlockChain().SubscribeRemovedLogsEvent(ch)
}

func (b *kokApiBackend) SubscribeChainEvent(ch chan<- core.ChainEvent) event.Subscription {
	return b.kok.BlockChain().SubscribeChainEvent(ch)
}

func (b *kokApiBackend) SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription {
	return b.kok.BlockChain().SubscribeChainHeadEvent(ch)
}

func (b *kokApiBackend) SubscribeLogsEvent(ch chan<- []*types.Log) event.Subscription {
	return b.kok.BlockChain().SubscribeLogsEvent(ch)
}

func (b *kokApiBackend) SendTx(ctx context.Context, signedTx *types.Transaction) error {
	return b.kok.txPool.AddLocal(signedTx)
}

func (b *kokApiBackend) GetPoolTransactions() (types.Transactions, error) {
	pending, err := b.kok.txPool.Pending()
	if err != nil {
		return nil, err
	}
	var txs types.Transactions
	for _, batch := range pending {
		txs = append(txs, batch...)
	}
	return txs, nil
}

func (b *kokApiBackend) GetPoolTransaction(hash common.Hash) *types.Transaction {
	return b.kok.txPool.Get(hash)
}

func (b *kokApiBackend) GetPoolNonce(ctx context.Context, addr common.Address) (uint64, error) {
	return b.kok.txPool.State().GetNonce(addr), nil
}

func (b *kokApiBackend) Stats() (pending int, queued int) {
	return b.kok.txPool.Stats()
}

func (b *kokApiBackend) TxPoolContent() (map[common.Address]types.Transactions, map[common.Address]types.Transactions) {
	return b.kok.TxPool().Content()
}

func (b *kokApiBackend) SubscribeTxPreEvent(ch chan<- core.TxPreEvent) event.Subscription {
	return b.kok.TxPool().SubscribeTxPreEvent(ch)
}

func (b *kokApiBackend) Downloader() *downloader.Downloader {
	return b.kok.Downloader()
}

func (b *kokApiBackend) ProtocolVersion() int {
	return b.kok.kokVersion()
}

func (b *kokApiBackend) SuggestPrice(ctx context.Context) (*big.Int, error) {
	return b.gpo.SuggestPrice(ctx)
}

func (b *kokApiBackend) ChainDb() kokdb.Database {
	return b.kok.ChainDb()
}

func (b *kokApiBackend) EventMux() *event.TypeMux {
	return b.kok.EventMux()
}

func (b *kokApiBackend) AccountManager() *accounts.Manager {
	return b.kok.AccountManager()
}

func (b *kokApiBackend) BloomStatus() (uint64, uint64) {
	sections, _, _ := b.kok.bloomIndexer.Sections()
	return params.BloomBitsBlocks, sections
}

func (b *kokApiBackend) ServiceFilter(ctx context.Context, session *bloombits.MatcherSession) {
	for i := 0; i < bloomFilterThreads; i++ {
		go session.Multiplex(bloomRetrievalBatch, bloomRetrievalWait, b.kok.bloomRequests)
	}
}
