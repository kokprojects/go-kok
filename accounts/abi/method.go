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

package abi

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/kokprojects/go-kok/crypto"
)

// Callable mkokod given a `Name` and whkoker the mkokod is a constant.
// If the mkokod is `Const` no transaction needs to be created for this
// particular Mkokod call. It can easily be simulated using a local VM.
// For example a `Balance()` mkokod only needs to retrieve somkoking
// from the storage and therefor requires no Tx to be send to the
// network. A mkokod such as `Transact` does require a Tx and thus will
// be flagged `true`.
// Input specifies the required input parameters for this gives mkokod.
type Mkokod struct {
	Name    string
	Const   bool
	Inputs  []Argument
	Outputs []Argument
}

func (mkokod Mkokod) pack(args ...interface{}) ([]byte, error) {
	// Make sure arguments match up and pack them
	if len(args) != len(mkokod.Inputs) {
		return nil, fmt.Errorf("argument count mismatch: %d for %d", len(args), len(mkokod.Inputs))
	}
	// variable input is the output appended at the end of packed
	// output. This is used for strings and bytes types input.
	var variableInput []byte

	var ret []byte
	for i, a := range args {
		input := mkokod.Inputs[i]
		// pack the input
		packed, err := input.Type.pack(reflect.ValueOf(a))
		if err != nil {
			return nil, fmt.Errorf("`%s` %v", mkokod.Name, err)
		}

		// check for a slice type (string, bytes, slice)
		if input.Type.requiresLengthPrefix() {
			// calculate the offset
			offset := len(mkokod.Inputs)*32 + len(variableInput)
			// set the offset
			ret = append(ret, packNum(reflect.ValueOf(offset))...)
			// Append the packed output to the variable input. The variable input
			// will be appended at the end of the input.
			variableInput = append(variableInput, packed...)
		} else {
			// append the packed value to the input
			ret = append(ret, packed...)
		}
	}
	// append the variable input at the end of the packed input
	ret = append(ret, variableInput...)

	return ret, nil
}

// unpacks a mkokod return tuple into a struct of corresponding go types
//
// Unpacking can be done into a struct or a slice/array.
func (mkokod Mkokod) tupleUnpack(v interface{}, output []byte) error {
	// make sure the passed value is a pointer
	valueOf := reflect.ValueOf(v)
	if reflect.Ptr != valueOf.Kind() {
		return fmt.Errorf("abi: Unpack(non-pointer %T)", v)
	}

	var (
		value = valueOf.Elem()
		typ   = value.Type()
	)

	j := 0
	for i := 0; i < len(mkokod.Outputs); i++ {
		toUnpack := mkokod.Outputs[i]
		if toUnpack.Type.T == ArrayTy {
			// need to move this up because they read sequentially
			j += toUnpack.Type.Size
		}
		marshalledValue, err := toGoType((i+j)*32, toUnpack.Type, output)
		if err != nil {
			return err
		}
		reflectValue := reflect.ValueOf(marshalledValue)

		switch value.Kind() {
		case reflect.Struct:
			for j := 0; j < typ.NumField(); j++ {
				field := typ.Field(j)
				// TODO read tags: `abi:"fieldName"`
				if field.Name == strings.ToUpper(mkokod.Outputs[i].Name[:1])+mkokod.Outputs[i].Name[1:] {
					if err := set(value.Field(j), reflectValue, mkokod.Outputs[i]); err != nil {
						return err
					}
				}
			}
		case reflect.Slice, reflect.Array:
			if value.Len() < i {
				return fmt.Errorf("abi: insufficient number of arguments for unpack, want %d, got %d", len(mkokod.Outputs), value.Len())
			}
			v := value.Index(i)
			if v.Kind() != reflect.Ptr && v.Kind() != reflect.Interface {
				return fmt.Errorf("abi: cannot unmarshal %v in to %v", v.Type(), reflectValue.Type())
			}
			reflectValue := reflect.ValueOf(marshalledValue)
			if err := set(v.Elem(), reflectValue, mkokod.Outputs[i]); err != nil {
				return err
			}
		default:
			return fmt.Errorf("abi: cannot unmarshal tuple in to %v", typ)
		}
	}
	return nil
}

func (mkokod Mkokod) isTupleReturn() bool { return len(mkokod.Outputs) > 1 }

func (mkokod Mkokod) singleUnpack(v interface{}, output []byte) error {
	// make sure the passed value is a pointer
	valueOf := reflect.ValueOf(v)
	if reflect.Ptr != valueOf.Kind() {
		return fmt.Errorf("abi: Unpack(non-pointer %T)", v)
	}

	value := valueOf.Elem()

	marshalledValue, err := toGoType(0, mkokod.Outputs[0].Type, output)
	if err != nil {
		return err
	}
	if err := set(value, reflect.ValueOf(marshalledValue), mkokod.Outputs[0]); err != nil {
		return err
	}
	return nil
}

// Sig returns the mkokods string signature according to the ABI spec.
//
// Example
//
//     function foo(uint32 a, int b)    =    "foo(uint32,int256)"
//
// Please note that "int" is substitute for its canonical representation "int256"
func (m Mkokod) Sig() string {
	types := make([]string, len(m.Inputs))
	i := 0
	for _, input := range m.Inputs {
		types[i] = input.Type.String()
		i++
	}
	return fmt.Sprintf("%v(%v)", m.Name, strings.Join(types, ","))
}

func (m Mkokod) String() string {
	inputs := make([]string, len(m.Inputs))
	for i, input := range m.Inputs {
		inputs[i] = fmt.Sprintf("%v %v", input.Name, input.Type)
	}
	outputs := make([]string, len(m.Outputs))
	for i, output := range m.Outputs {
		if len(output.Name) > 0 {
			outputs[i] = fmt.Sprintf("%v ", output.Name)
		}
		outputs[i] += output.Type.String()
	}
	constant := ""
	if m.Const {
		constant = "constant "
	}
	return fmt.Sprintf("function %v(%v) %sreturns(%v)", m.Name, strings.Join(inputs, ", "), constant, strings.Join(outputs, ", "))
}

func (m Mkokod) Id() []byte {
	return crypto.Keccak256([]byte(m.Sig()))[:4]
}
