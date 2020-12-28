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

// Contains the metrics collected by the fetcher.

package fetcher

import (
	"github.com/kokprojects/go-kok/metrics"
)

var (
	propAnnounceInMeter   = metrics.NewMeter("kok/fetcher/prop/announces/in")
	propAnnounceOutTimer  = metrics.NewTimer("kok/fetcher/prop/announces/out")
	propAnnounceDropMeter = metrics.NewMeter("kok/fetcher/prop/announces/drop")
	propAnnounceDOSMeter  = metrics.NewMeter("kok/fetcher/prop/announces/dos")

	propBroadcastInMeter   = metrics.NewMeter("kok/fetcher/prop/broadcasts/in")
	propBroadcastOutTimer  = metrics.NewTimer("kok/fetcher/prop/broadcasts/out")
	propBroadcastDropMeter = metrics.NewMeter("kok/fetcher/prop/broadcasts/drop")
	propBroadcastDOSMeter  = metrics.NewMeter("kok/fetcher/prop/broadcasts/dos")

	headerFetchMeter = metrics.NewMeter("kok/fetcher/fetch/headers")
	bodyFetchMeter   = metrics.NewMeter("kok/fetcher/fetch/bodies")

	headerFilterInMeter  = metrics.NewMeter("kok/fetcher/filter/headers/in")
	headerFilterOutMeter = metrics.NewMeter("kok/fetcher/filter/headers/out")
	bodyFilterInMeter    = metrics.NewMeter("kok/fetcher/filter/bodies/in")
	bodyFilterOutMeter   = metrics.NewMeter("kok/fetcher/filter/bodies/out")
)
