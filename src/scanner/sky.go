// Package scanner scans bitcoin blockchain and check all transactions
// to see if there are addresses in vout that can match our deposit addresses.
// If found, then generate an event and push to deposit event channel
//
// current scanner doesn't support reconnect after btcd shutdown, if
// any error occur when call btcd apis, the scan service will be closed.
package scanner

import (
	"time"

	"github.com/sirupsen/logrus"

	"github.com/skycoin/skycoin/src/api/webrpc"
	"github.com/skycoin/skycoin/src/util/droplet"
	"github.com/skycoin/skycoin/src/visor"
)

// SKYScanner blockchain scanner to check if there're deposit coins
type SKYScanner struct {
	log       logrus.FieldLogger
	skyClient SkyRPCClient
	// Deposit value channel, exposed by public API, intended for public consumption
	Base CommonScanner
}

// SkyClient implements the SKYRpcClient interface
type SkyClient struct {
	c *webrpc.Client
}

// NewSkyClient create a new skyclient instance
func NewSkyClient(addr string) *SkyClient {
	skyClient := SkyClient{
		c: &webrpc.Client{Addr: addr},
	}
	return &skyClient
}

// NewSKYScanner creates scanner instance
func NewSKYScanner(log logrus.FieldLogger, store Storer, sky SkyRPCClient, cfg Config) (*SKYScanner, error) {
	bs := NewBaseScanner(store, log.WithField("prefix", "scanner.sky"), CoinTypeSKY, cfg)

	return &SKYScanner{
		skyClient: sky,
		log:       log.WithField("prefix", "scanner.sky"),
		Base:      bs,
	}, nil
}

// Run begins the SKYScanner
func (s *SKYScanner) Run() error {
	return s.Base.Run(s.skyClient.GetBlockCount, s.getBlockAtHeight, s.waitForNextBlock, s.scanBlock)
}

// Shutdown shutdown the scanner
func (s *SKYScanner) Shutdown() {
	s.log.Info("Closing SKY scanner")
	s.skyClient.Shutdown()
	s.Base.Shutdown()
	s.log.Info("Waiting for SKY scanner to stop")
	s.log.Info("SKY scanner stopped")
}

// scanBlock scans for a new SKY block every ScanPeriod.
// When a new block is found, it compares the block against our scanning
// deposit addresses. If a matching deposit is found, it saves it to the DB.
func (s *SKYScanner) scanBlock(block *CommonBlock) (int, error) {
	log := s.log.WithField("hash", block.Hash)
	log = log.WithField("height", block.Height)

	log.Debug("Scanning block")

	dvs, err := s.Base.GetStorer().ScanBlock(block, CoinTypeSKY)
	if err != nil {
		log.WithError(err).Error("store.ScanBlock failed")
		return 0, err
	}

	log = log.WithField("scannedDeposits", len(dvs))
	log.Infof("Counted %d deposits from block", len(dvs))

	n := 0
	for _, dv := range dvs {
		select {
		case s.Base.GetScannedDepositChan() <- dv:
			n++
		case <-s.Base.GetQuitChan():
			return n, errQuit
		}
	}

	return n, nil
}

//GetBlockCount returns skycoin block count
func (sc *SkyClient) GetBlockCount() (int64, error) {
	// get the last block
	lastBlock, err := sc.c.GetLastBlocks(1)
	if err != nil {
		logrus.WithError(err).Error("skyClient.GetBlockCount failed")
		return 0, err
	}

	// block seq of last block is the block count
	blockCnt := lastBlock.Blocks[0].Head.BkSeq
	return int64(blockCnt), nil
}

// GetBlockVerboseTx returns skycoin block data for a give height
func (sc *SkyClient) GetBlockVerboseTx(seq uint64) (*visor.ReadableBlock, error) {
	block, err := sc.c.GetBlocksBySeq([]uint64{seq})
	if err != nil {
		logrus.WithError(err).Error("skyClient.GetBlockVerboseTx failed")
		return nil, err
	}

	return &block.Blocks[0], nil
}

// Shutdown placeholder
func (sc *SkyClient) Shutdown() {
}

// getBlockAtHeight returns that block at a specific height
func (s *SKYScanner) getBlockAtHeight(height int64) (*CommonBlock, error) {
	block, err := s.skyClient.GetBlockVerboseTx(uint64(height))
	if err != nil {
		return nil, err
	}
	return skyBlock2CommonBlock(block)
}

// skyBlock2CommonBlock convert bitcoin block to common block
func skyBlock2CommonBlock(block *visor.ReadableBlock) (*CommonBlock, error) {
	cb := CommonBlock{}
	cb.Hash = block.Head.BlockHash
	cb.Height = int64(block.Head.BkSeq)
	cb.RawTx = make([]CommonTx, 0, len(block.Body.Transactions))
	for _, tx := range block.Body.Transactions {
		cbTx := CommonTx{}
		cbTx.Txid = tx.Hash
		cbTx.Vout = make([]CommonVout, 0, len(tx.Out))
		for _, v := range tx.Out {
			// internally skycoins are always represented as droplets
			amt, err := droplet.FromString(v.Coins)
			if err != nil {
				return nil, err
			}
			cv := CommonVout{}
			cv.Value = int64(amt)
			cv.Addresses = []string{v.Address}
			cbTx.Vout = append(cbTx.Vout, cv)
		}
		cb.RawTx = append(cb.RawTx, cbTx)
	}

	return &cb, nil
}

// getNextBlock returns the next block from another block, return nil if next block does not exist
func (s *SKYScanner) getNextBlock(seq int64) (*CommonBlock, error) {
	b, err := s.skyClient.GetBlockVerboseTx(uint64(seq + 1))
	if err != nil {
		return nil, err
	}
	return skyBlock2CommonBlock(b)
}

// waitForNextBlock scans for the next block until it is available
func (s *SKYScanner) waitForNextBlock(block *CommonBlock) (*CommonBlock, error) {
	log := s.log.WithField("blockHash", block.Hash)
	log = log.WithField("blockHeight", block.Height)
	log.Debug("Waiting for the next block")

	for {
		nextBlock, err := s.getNextBlock(block.Height)
		if err != nil {
			log.WithError(err).Error("getNextBlock failed")
		}
		if nextBlock == nil {
			log.Debug("No new block yet")
		}
		if err != nil || nextBlock == nil {
			select {
			case <-s.Base.GetQuitChan():
				return nil, errQuit
			case <-time.After(s.Base.GetScanPeriod()):
				continue
			}
		}

		log.WithFields(logrus.Fields{
			"hash":   nextBlock.Hash,
			"height": nextBlock.Height,
		}).Debug("Found nextBlock")

		return nextBlock, nil
	}
}

// AddScanAddress adds new scan address
func (s *SKYScanner) AddScanAddress(addr, coinType string) error {
	return s.Base.GetStorer().AddScanAddress(addr, coinType)
}

// GetScanAddresses returns the deposit addresses that need to scan
func (s *SKYScanner) GetScanAddresses() ([]string, error) {
	return s.Base.GetStorer().GetScanAddresses(CoinTypeSKY)
}

//GetDeposit returns channel of depositnote
func (s *SKYScanner) GetDeposit() <-chan DepositNote {
	return s.Base.GetDeposit()
}
