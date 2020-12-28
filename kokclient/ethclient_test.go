// Copyright 2016 The go-kokereum Authors
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

package kokclient

import "github.com/kokprojects/go-kok"

// Verify that Client implements the kokereum interfaces.
var (
	_ = kokereum.ChainReader(&Client{})
	_ = kokereum.TransactionReader(&Client{})
	_ = kokereum.ChainStateReader(&Client{})
	_ = kokereum.ChainSyncReader(&Client{})
	_ = kokereum.ContractCaller(&Client{})
	_ = kokereum.GasEstimator(&Client{})
	_ = kokereum.GasPricer(&Client{})
	_ = kokereum.LogFilterer(&Client{})
	_ = kokereum.PendingStateReader(&Client{})
	// _ = kokereum.PendingStateEventer(&Client{})
	_ = kokereum.PendingContractCaller(&Client{})
)
