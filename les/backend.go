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

// Package les implements the Light kokereum Subprotocol.
package les

import (
	"fmt"
	"sync"
	"time"

	"github.com/kokprojects/go-kok/accounts"
	"github.com/kokprojects/go-kok/common"
	"github.com/kokprojects/go-kok/common/hexutil"
	"github.com/kokprojects/go-kok/consensus"
	"github.com/kokprojects/go-kok/consensus/dpos"
	"github.com/kokprojects/go-kok/core"
	"github.com/kokprojects/go-kok/core/bloombits"
	"github.com/kokprojects/go-kok/core/types"
	"github.com/kokprojects/go-kok/kok"
	"github.com/kokprojects/go-kok/kok/downloader"
	"github.com/kokprojects/go-kok/kok/filters"
	"github.com/kokprojects/go-kok/kok/gasprice"
	"github.com/kokprojects/go-kok/kokdb"
	"github.com/kokprojects/go-kok/event"
	"github.com/kokprojects/go-kok/internal/kokapi"
	"github.com/kokprojects/go-kok/light"
	"github.com/kokprojects/go-kok/log"
	"github.com/kokprojects/go-kok/node"
	"github.com/kokprojects/go-kok/p2p"
	"github.com/kokprojects/go-kok/p2p/discv5"
	"github.com/kokprojects/go-kok/params"
	rpc "github.com/kokprojects/go-kok/rpc"
)

type Lightkokereum struct {
	odr         *LesOdr
	relay       *LesTxRelay
	chainConfig *params.ChainConfig
	// Channel for shutting down the service
	shutdownChan chan bool
	// Handlers
	peers           *peerSet
	txPool          *light.TxPool
	blockchain      *light.LightChain
	protocolManager *ProtocolManager
	serverPool      *serverPool
	reqDist         *requestDistributor
	retriever       *retrieveManager
	// DB interfaces
	chainDb kokdb.Database // Block chain database

	bloomRequests                              chan chan *bloombits.Retrieval // Channel receiving bloom data retrieval requests
	bloomIndexer, chtIndexer, bloomTrieIndexer *core.ChainIndexer

	ApiBackend *LesApiBackend

	eventMux       *event.TypeMux
	engine         consensus.Engine
	accountManager *accounts.Manager

	networkId     uint64
	netRPCService *kokapi.PublicNetAPI

	wg sync.WaitGroup
}

func New(ctx *node.ServiceContext, config *kok.Config) (*Lightkokereum, error) {
	chainDb, err := kok.CreateDB(ctx, config, "lightchaindata")
	if err != nil {
		return nil, err
	}
	chainConfig, genesisHash, genesisErr := core.SetupGenesisBlock(chainDb, config.Genesis)
	if _, isCompat := genesisErr.(*params.ConfigCompatError); genesisErr != nil && !isCompat {
		return nil, genesisErr
	}
	log.Info("Initialised chain configuration", "config", chainConfig)

	peers := newPeerSet()
	quitSync := make(chan struct{})

	lkok := &Lightkokereum{
		chainConfig:      chainConfig,
		chainDb:          chainDb,
		eventMux:         ctx.EventMux,
		peers:            peers,
		reqDist:          newRequestDistributor(peers, quitSync),
		accountManager:   ctx.AccountManager,
		engine:           dpos.New(chainConfig.Dpos, chainDb),
		shutdownChan:     make(chan bool),
		networkId:        config.NetworkId,
		bloomRequests:    make(chan chan *bloombits.Retrieval),
		bloomIndexer:     kok.NewBloomIndexer(chainDb, light.BloomTrieFrequency),
		chtIndexer:       light.NewChtIndexer(chainDb, true),
		bloomTrieIndexer: light.NewBloomTrieIndexer(chainDb, true),
	}

	lkok.relay = NewLesTxRelay(peers, lkok.reqDist)
	lkok.serverPool = newServerPool(chainDb, quitSync, &lkok.wg)
	lkok.retriever = newRetrieveManager(peers, lkok.reqDist, lkok.serverPool)
	lkok.odr = NewLesOdr(chainDb, lkok.chtIndexer, lkok.bloomTrieIndexer, lkok.bloomIndexer, lkok.retriever)
	if lkok.blockchain, err = light.NewLightChain(lkok.odr, lkok.chainConfig, lkok.engine); err != nil {
		return nil, err
	}
	lkok.bloomIndexer.Start(lkok.blockchain)
	// Rewind the chain in case of an incompatible config upgrade.
	if compat, ok := genesisErr.(*params.ConfigCompatError); ok {
		log.Warn("Rewinding chain to upgrade configuration", "err", compat)
		lkok.blockchain.Skokead(compat.RewindTo)
		core.WriteChainConfig(chainDb, genesisHash, chainConfig)
	}

	lkok.txPool = light.NewTxPool(lkok.chainConfig, lkok.blockchain, lkok.relay)
	if lkok.protocolManager, err = NewProtocolManager(lkok.chainConfig, true, ClientProtocolVersions, config.NetworkId, lkok.eventMux, lkok.engine, lkok.peers, lkok.blockchain, nil, chainDb, lkok.odr, lkok.relay, quitSync, &lkok.wg); err != nil {
		return nil, err
	}
	lkok.ApiBackend = &LesApiBackend{lkok, nil}
	gpoParams := config.GPO
	if gpoParams.Default == nil {
		gpoParams.Default = config.GasPrice
	}
	lkok.ApiBackend.gpo = gasprice.NewOracle(lkok.ApiBackend, gpoParams)
	return lkok, nil
}

func lesTopic(genesisHash common.Hash, protocolVersion uint) discv5.Topic {
	var name string
	switch protocolVersion {
	case lpv1:
		name = "LES"
	case lpv2:
		name = "LES2"
	default:
		panic(nil)
	}
	return discv5.Topic(name + "@" + common.Bytes2Hex(genesisHash.Bytes()[0:8]))
}

type LightDummyAPI struct{}

// Coinbase is the address that mining rewards will be send to
func (s *LightDummyAPI) Coinbase() (common.Address, error) {
	return common.Address{}, fmt.Errorf("not supported")
}

// Hashrate returns the POW hashrate
func (s *LightDummyAPI) Hashrate() hexutil.Uint {
	return 0
}

// Mining returns an indication if this node is currently mining.
func (s *LightDummyAPI) Mining() bool {
	return false
}

// APIs returns the collection of RPC services the kokereum package offers.
// NOTE, some of these services probably need to be moved to somewhere else.
func (s *Lightkokereum) APIs() []rpc.API {
	return append(kokapi.GetAPIs(s.ApiBackend), []rpc.API{
		{
			Namespace: "kok",
			Version:   "1.0",
			Service:   &LightDummyAPI{},
			Public:    true,
		}, {
			Namespace: "kok",
			Version:   "1.0",
			Service:   downloader.NewPublicDownloaderAPI(s.protocolManager.downloader, s.eventMux),
			Public:    true,
		}, {
			Namespace: "kok",
			Version:   "1.0",
			Service:   filters.NewPublicFilterAPI(s.ApiBackend, true),
			Public:    true,
		}, {
			Namespace: "net",
			Version:   "1.0",
			Service:   s.netRPCService,
			Public:    true,
		},
	}...)
}

func (s *Lightkokereum) ResetWithGenesisBlock(gb *types.Block) {
	s.blockchain.ResetWithGenesisBlock(gb)
}

func (s *Lightkokereum) BlockChain() *light.LightChain      { return s.blockchain }
func (s *Lightkokereum) TxPool() *light.TxPool              { return s.txPool }
func (s *Lightkokereum) Engine() consensus.Engine           { return s.engine }
func (s *Lightkokereum) LesVersion() int                    { return int(s.protocolManager.SubProtocols[0].Version) }
func (s *Lightkokereum) Downloader() *downloader.Downloader { return s.protocolManager.downloader }
func (s *Lightkokereum) EventMux() *event.TypeMux           { return s.eventMux }

// Protocols implements node.Service, returning all the currently configured
// network protocols to start.
func (s *Lightkokereum) Protocols() []p2p.Protocol {
	return s.protocolManager.SubProtocols
}

// Start implements node.Service, starting all internal goroutines needed by the
// kokereum protocol implementation.
func (s *Lightkokereum) Start(srvr *p2p.Server) error {
	s.startBloomHandlers()
	log.Warn("Light client mode is an experimental feature")
	s.netRPCService = kokapi.NewPublicNetAPI(srvr, s.networkId)
	// search the topic belonging to the oldest supported protocol because
	// servers always advertise all supported protocols
	protocolVersion := ClientProtocolVersions[len(ClientProtocolVersions)-1]
	s.serverPool.start(srvr, lesTopic(s.blockchain.Genesis().Hash(), protocolVersion))
	s.protocolManager.Start()
	return nil
}

// Stop implements node.Service, terminating all internal goroutines used by the
// kokereum protocol.
func (s *Lightkokereum) Stop() error {
	s.odr.Stop()
	if s.bloomIndexer != nil {
		s.bloomIndexer.Close()
	}
	if s.chtIndexer != nil {
		s.chtIndexer.Close()
	}
	if s.bloomTrieIndexer != nil {
		s.bloomTrieIndexer.Close()
	}
	s.blockchain.Stop()
	s.protocolManager.Stop()
	s.txPool.Stop()

	s.eventMux.Stop()

	time.Sleep(time.Millisecond * 200)
	s.chainDb.Close()
	close(s.shutdownChan)

	return nil
}
