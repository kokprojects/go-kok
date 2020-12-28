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
	"encoding/json"
	"fmt"
	"io"
)

// The ABI holds information about a contract's context and available
// invokable mkokods. It will allow you to type check function calls and
// packs data accordingly.
type ABI struct {
	Constructor Mkokod
	Mkokods     map[string]Mkokod
	Events      map[string]Event
}

// JSON returns a parsed ABI interface and error if it failed.
func JSON(reader io.Reader) (ABI, error) {
	dec := json.NewDecoder(reader)

	var abi ABI
	if err := dec.Decode(&abi); err != nil {
		return ABI{}, err
	}

	return abi, nil
}

// Pack the given mkokod name to conform the ABI. Mkokod call's data
// will consist of mkokod_id, args0, arg1, ... argN. Mkokod id consists
// of 4 bytes and arguments are all 32 bytes.
// Mkokod ids are created from the first 4 bytes of the hash of the
// mkokods string signature. (signature = baz(uint32,string32))
func (abi ABI) Pack(name string, args ...interface{}) ([]byte, error) {
	// Fetch the ABI of the requested mkokod
	var mkokod Mkokod

	if name == "" {
		mkokod = abi.Constructor
	} else {
		m, exist := abi.Mkokods[name]
		if !exist {
			return nil, fmt.Errorf("mkokod '%s' not found", name)
		}
		mkokod = m
	}
	arguments, err := mkokod.pack(args...)
	if err != nil {
		return nil, err
	}
	// Pack up the mkokod ID too if not a constructor and return
	if name == "" {
		return arguments, nil
	}
	return append(mkokod.Id(), arguments...), nil
}

// Unpack output in v according to the abi specification
func (abi ABI) Unpack(v interface{}, name string, output []byte) (err error) {
	if err = bytesAreProper(output); err != nil {
		return err
	}
	// since there can't be naming collisions with contracts and events,
	// we need to decide whkoker we're calling a mkokod or an event
	var unpack unpacker
	if mkokod, ok := abi.Mkokods[name]; ok {
		unpack = mkokod
	} else if event, ok := abi.Events[name]; ok {
		unpack = event
	} else {
		return fmt.Errorf("abi: could not locate named mkokod or event.")
	}

	// requires a struct to unpack into for a tuple return...
	if unpack.isTupleReturn() {
		return unpack.tupleUnpack(v, output)
	}
	return unpack.singleUnpack(v, output)
}

func (abi *ABI) UnmarshalJSON(data []byte) error {
	var fields []struct {
		Type      string
		Name      string
		Constant  bool
		Indexed   bool
		Anonymous bool
		Inputs    []Argument
		Outputs   []Argument
	}

	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}

	abi.Mkokods = make(map[string]Mkokod)
	abi.Events = make(map[string]Event)
	for _, field := range fields {
		switch field.Type {
		case "constructor":
			abi.Constructor = Mkokod{
				Inputs: field.Inputs,
			}
		// empty defaults to function according to the abi spec
		case "function", "":
			abi.Mkokods[field.Name] = Mkokod{
				Name:    field.Name,
				Const:   field.Constant,
				Inputs:  field.Inputs,
				Outputs: field.Outputs,
			}
		case "event":
			abi.Events[field.Name] = Event{
				Name:      field.Name,
				Anonymous: field.Anonymous,
				Inputs:    field.Inputs,
			}
		}
	}

	return nil
}
