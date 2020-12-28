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

// Package kok implements the kokereum protocol.
package kok

import (
	"errors"
	"fmt"
	"math/big"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/kokprojects/go-kok/accounts"
	"github.com/kokprojects/go-kok/common"
	"github.com/kokprojects/go-kok/common/hexutil"
	"github.com/kokprojects/go-kok/consensus"
	"github.com/kokprojects/go-kok/consensus/dpos"
	"github.com/kokprojects/go-kok/core"
	"github.com/kokprojects/go-kok/core/bloombits"
	"github.com/kokprojects/go-kok/core/types"
	"github.com/kokprojects/go-kok/core/vm"
	"github.com/kokprojects/go-kok/kok/downloader"
	"github.com/kokprojects/go-kok/kok/filters"
	"github.com/kokprojects/go-kok/kok/gasprice"
	"github.com/kokprojects/go-kok/kokdb"
	"github.com/kokprojects/go-kok/event"
	"github.com/kokprojects/go-kok/internal/kokapi"
	"github.com/kokprojects/go-kok/log"
	"github.com/kokprojects/go-kok/miner"
	"github.com/kokprojects/go-kok/node"
	"github.com/kokprojects/go-kok/p2p"
	"github.com/kokprojects/go-kok/params"
	"github.com/kokprojects/go-kok/rlp"
	"github.com/kokprojects/go-kok/rpc"
)

type LesServer interface {
	Start(srvr *p2p.Server)
	Stop()
	Protocols() []p2p.Protocol
	SetBloomBitsIndexer(bbIndexer *core.ChainIndexer)
}

// kokereum implements the kokereum full node service.
type kokereum struct {
	config      *Config
	chainConfig *params.ChainConfig

	// Channel for shutting down the service
	shutdownChan  chan bool    // Channel for shutting down the kokereum
	stopDbUpgrade func() error // stop chain db sequential key upgrade

	// Handlers
	txPool          *core.TxPool
	blockchain      *core.BlockChain
	protocolManager *ProtocolManager
	lesServer       LesServer

	// DB interfaces
	chainDb kokdb.Database // Block chain database

	eventMux       *event.TypeMux
	engine         consensus.Engine
	accountManager *accounts.Manager

	bloomRequests chan chan *bloombits.Retrieval // Channel receiving bloom data retrieval requests
	bloomIndexer  *core.ChainIndexer             // Bloom indexer operating during block imports

	ApiBackend *kokApiBackend

	miner     *miner.Miner
	gasPrice  *big.Int
	validator common.Address
	coinbase  common.Address

	networkId     uint64
	netRPCService *kokapi.PublicNetAPI

	lock sync.RWMutex // Protects the variadic fields (e.g. gas price and coinbase)
}

func (s *kokereum) AddLesServer(ls LesServer) {
	s.lesServer = ls
	ls.SetBloomBitsIndexer(s.bloomIndexer)
}

// New creates a new kokereum object (including the
// initialisation of the common kokereum object)
func New(ctx *node.ServiceContext, config *Config) (*kokereum, error) {
	if config.SyncMode == downloader.LightSync {
		return nil, errors.New("can't run kok.kokereum in light sync mode, use les.Lightkokereum")
	}
	if !config.SyncMode.IsValid() {
		return nil, fmt.Errorf("invalid sync mode %d", config.SyncMode)
	}
	chainDb, err := CreateDB(ctx, config, "chaindata")
	if err != nil {
		return nil, err
	}
	stopDbUpgrade := upgradeDeduplicateData(chainDb)
	chainConfig, genesisHash, genesisErr := core.SetupGenesisBlock(chainDb, config.Genesis)
	if _, ok := genesisErr.(*params.ConfigCompatError); genesisErr != nil && !ok {
		return nil, genesisErr
	}
	log.Info("Initialised chain configuration", "config", chainConfig)

	kok := &kokereum{
		config:         config,
		chainDb:        chainDb,
		chainConfig:    chainConfig,
		eventMux:       ctx.EventMux,
		accountManager: ctx.AccountManager,
		engine:         dpos.New(chainConfig.Dpos, chainDb),
		shutdownChan:   make(chan bool),
		stopDbUpgrade:  stopDbUpgrade,
		networkId:      config.NetworkId,
		gasPrice:       config.GasPrice,
		validator:      config.Validator,
		coinbase:       config.Coinbase,
		bloomRequests:  make(chan chan *bloombits.Retrieval),
		bloomIndexer:   NewBloomIndexer(chainDb, params.BloomBitsBlocks),
	}

	log.Info("Initialising kokereum protocol", "versions", ProtocolVersions, "network", config.NetworkId)

	if !config.SkipBcVersionCheck {
		bcVersion := core.GetBlockChainVersion(chainDb)
		if bcVersion != core.BlockChainVersion && bcVersion != 0 {
			return nil, fmt.Errorf("Blockchain DB version mismatch (%d / %d). Run gkok upgradedb.\n", bcVersion, core.BlockChainVersion)
		}
		core.WriteBlockChainVersion(chainDb, core.BlockChainVersion)
	}
	vmConfig := vm.Config{EnablePreimageRecording: config.EnablePreimageRecording}
	kok.blockchain, err = core.NewBlockChain(chainDb, kok.chainConfig, kok.engine, vmConfig)
	if err != nil {
		return nil, err
	}
	// Rewind the chain in case of an incompatible config upgrade.
	if compat, ok := genesisErr.(*params.ConfigCompatError); ok {
		log.Warn("Rewinding chain to upgrade configuration", "err", compat)
		kok.blockchain.Skokead(compat.RewindTo)
		core.WriteChainConfig(chainDb, genesisHash, chainConfig)
	}
	kok.bloomIndexer.Start(kok.blockchain)

	if config.TxPool.Journal != "" {
		config.TxPool.Journal = ctx.ResolvePath(config.TxPool.Journal)
	}
	kok.txPool = core.NewTxPool(config.TxPool, kok.chainConfig, kok.blockchain)

	if kok.protocolManager, err = NewProtocolManager(kok.chainConfig, config.SyncMode, config.NetworkId, kok.eventMux, kok.txPool, kok.engine, kok.blockchain, chainDb); err != nil {
		return nil, err
	}
	kok.miner = miner.New(kok, kok.chainConfig, kok.EventMux(), kok.engine)
	kok.miner.SetExtra(makeExtraData(config.ExtraData))

	kok.ApiBackend = &kokApiBackend{kok, nil}
	gpoParams := config.GPO
	if gpoParams.Default == nil {
		gpoParams.Default = config.GasPrice
	}
	kok.ApiBackend.gpo = gasprice.NewOracle(kok.ApiBackend, gpoParams)

	return kok, nil
}

func makeExtraData(extra []byte) []byte {
	if len(extra) == 0 {
		// create default extradata
		extra, _ = rlp.EncodeToBytes([]interface{}{
			uint(params.VersionMajor<<16 | params.VersionMinor<<8 | params.VersionPatch),
			"gkok",
			runtime.Version(),
			runtime.GOOS,
		})
		log.Warn(hexutil.Encode(extra))
	}
	if uint64(len(extra)) > params.MaximumExtraDataSize {
		log.Warn("Miner extra data exceed limit", "extra", hexutil.Bytes(extra), "limit", params.MaximumExtraDataSize)
		extra = nil
	}
	return extra
}

// CreateDB creates the chain database.
func CreateDB(ctx *node.ServiceContext, config *Config, name string) (kokdb.Database, error) {
	db, err := ctx.OpenDatabase(name, config.DatabaseCache, config.DatabaseHandles)
	if err != nil {
		return nil, err
	}
	if db, ok := db.(*kokdb.LDBDatabase); ok {
		db.Meter("kok/db/chaindata/")
	}
	return db, nil
}

// APIs returns the collection of RPC services the kokereum package offers.
// NOTE, some of these services probably need to be moved to somewhere else.
func (s *kokereum) APIs() []rpc.API {
	apis := kokapi.GetAPIs(s.ApiBackend)

	// Append any APIs exposed explicitly by the consensus engine
	apis = append(apis, s.engine.APIs(s.BlockChain())...)

	// Append all the local APIs and return
	return append(apis, []rpc.API{
		{
			Namespace: "kok",
			Version:   "1.0",
			Service:   NewPublickokereumAPI(s),
			Public:    true,
		}, {
			Namespace: "kok",
			Version:   "1.0",
			Service:   NewPublicMinerAPI(s),
			Public:    true,
		}, {
			Namespace: "kok",
			Version:   "1.0",
			Service:   downloader.NewPublicDownloaderAPI(s.protocolManager.downloader, s.eventMux),
			Public:    true,
		}, {
			Namespace: "miner",
			Version:   "1.0",
			Service:   NewPrivateMinerAPI(s),
			Public:    false,
		}, {
			Namespace: "kok",
			Version:   "1.0",
			Service:   filters.NewPublicFilterAPI(s.ApiBackend, false),
			Public:    true,
		}, {
			Namespace: "admin",
			Version:   "1.0",
			Service:   NewPrivateAdminAPI(s),
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPublicDebugAPI(s),
			Public:    true,
		}, {
			Namespace: "debug",
			Version:   "1.0",
			Service:   NewPrivateDebugAPI(s.chainConfig, s),
		}, {
			Namespace: "net",
			Version:   "1.0",
			Service:   s.netRPCService,
			Public:    true,
		},
	}...)
}

func (s *kokereum) ResetWithGenesisBlock(gb *types.Block) {
	s.blockchain.ResetWithGenesisBlock(gb)
}

func (s *kokereum) Validator() (validator common.Address, err error) {
	s.lock.RLock()
	validator = s.validator
	s.lock.RUnlock()

	if validator != (common.Address{}) {
		return validator, nil
	}
	if wallets := s.AccountManager().Wallets(); len(wallets) > 0 {
		if accounts := wallets[0].Accounts(); len(accounts) > 0 {
			return accounts[0].Address, nil
		}
	}
	return common.Address{}, fmt.Errorf("validator address must be explicitly specified")
}

// set in js console via admin interface or wrapper from cli flags
func (self *kokereum) SetValidator(validator common.Address) {
	self.lock.Lock()
	self.validator = validator
	self.lock.Unlock()
}

func (s *kokereum) Coinbase() (eb common.Address, err error) {
	s.lock.RLock()
	coinbase := s.coinbase
	s.lock.RUnlock()

	if coinbase != (common.Address{}) {
		return coinbase, nil
	}
	if wallets := s.AccountManager().Wallets(); len(wallets) > 0 {
		if accounts := wallets[0].Accounts(); len(accounts) > 0 {
			return accounts[0].Address, nil
		}
	}
	return common.Address{}, fmt.Errorf("coinbase address must be explicitly specified")
}

// set in js console via admin interface or wrapper from cli flags
func (self *kokereum) SetCoinbase(coinbase common.Address) {
	self.lock.Lock()
	self.coinbase = coinbase
	self.lock.Unlock()

	self.miner.SetCoinbase(coinbase)
}

func (s *kokereum) StartMining(local bool) error {
	validator, err := s.Validator()
	if err != nil {
		log.Error("Cannot start mining without validator", "err", err)
		return fmt.Errorf("validator missing: %v", err)
	}
	cb, err := s.Coinbase()
	if err != nil {
		log.Error("Cannot start mining without coinbase", "err", err)
		return fmt.Errorf("coinbase missing: %v", err)
	}

	if dpos, ok := s.engine.(*dpos.Dpos); ok {
		wallet, err := s.accountManager.Find(accounts.Account{Address: validator})
		if wallet == nil || err != nil {
			log.Error("Coinbase account unavailable locally", "err", err)
			return fmt.Errorf("signer missing: %v", err)
		}
		dpos.Authorize(validator, wallet.SignHash)
	}
	if local {
		// If local (CPU) mining is started, we can disable the transaction rejection
		// mechanism introduced to speed sync times. CPU mining on mainnet is ludicrous
		// so noone will ever hit this path, whereas marking sync done on CPU mining
		// will ensure that private networks work in single miner mode too.
		atomic.StoreUint32(&s.protocolManager.acceptTxs, 1)
	}
	go s.miner.Start(cb)
	return nil
}

func (s *kokereum) StopMining()         { s.miner.Stop() }
func (s *kokereum) IsMining() bool      { return s.miner.Mining() }
func (s *kokereum) Miner() *miner.Miner { return s.miner }

func (s *kokereum) AccountManager() *accounts.Manager  { return s.accountManager }
func (s *kokereum) BlockChain() *core.BlockChain       { return s.blockchain }
func (s *kokereum) TxPool() *core.TxPool               { return s.txPool }
func (s *kokereum) EventMux() *event.TypeMux           { return s.eventMux }
func (s *kokereum) Engine() consensus.Engine           { return s.engine }
func (s *kokereum) ChainDb() kokdb.Database            { return s.chainDb }
func (s *kokereum) IsListening() bool                  { return true } // Always listening
func (s *kokereum) kokVersion() int                    { return int(s.protocolManager.SubProtocols[0].Version) }
func (s *kokereum) NetVersion() uint64                 { return s.networkId }
func (s *kokereum) Downloader() *downloader.Downloader { return s.protocolManager.downloader }

// Protocols implements node.Service, returning all the currently configured
// network protocols to start.
func (s *kokereum) Protocols() []p2p.Protocol {
	if s.lesServer == nil {
		return s.protocolManager.SubProtocols
	}
	return append(s.protocolManager.SubProtocols, s.lesServer.Protocols()...)
}

// Start implements node.Service, starting all internal goroutines needed by the
// kokereum protocol implementation.
func (s *kokereum) Start(srvr *p2p.Server) error {
	// Start the bloom bits servicing goroutines
	s.startBloomHandlers()

	// Start the RPC service
	s.netRPCService = kokapi.NewPublicNetAPI(srvr, s.NetVersion())

	// Figure out a max peers count based on the server limits
	maxPeers := srvr.MaxPeers
	if s.config.LightServ > 0 {
		maxPeers -= s.config.LightPeers
		if maxPeers < srvr.MaxPeers/2 {
			maxPeers = srvr.MaxPeers / 2
		}
	}
	// Start the networking layer and the light server if requested
	s.protocolManager.Start(maxPeers)
	if s.lesServer != nil {
		s.lesServer.Start(srvr)
	}
	return nil
}

// Stop implements node.Service, terminating all internal goroutines used by the
// kokereum protocol.
func (s *kokereum) Stop() error {
	if s.stopDbUpgrade != nil {
		s.stopDbUpgrade()
	}
	s.bloomIndexer.Close()
	s.blockchain.Stop()
	s.protocolManager.Stop()
	if s.lesServer != nil {
		s.lesServer.Stop()
	}
	s.txPool.Stop()
	s.miner.Stop()
	s.eventMux.Stop()

	s.chainDb.Close()
	close(s.shutdownChan)

	return nil
}
