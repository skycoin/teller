// Package scanner scans bitcoin blockchain and check all transactions
// to see if there are addresses in vout that can match our deposit addresses.
// If found, then generate an event and push to deposit event channel
//
// current scanner doesn't support reconnect after btcd shutdown, if
// any error occur when call btcd apis, the scan service will be closed.
package scanner

import (
	"errors"
	"time"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/sirupsen/logrus"
)

var (
	errQuit = errors.New("Scanner quit")

	// ErrBtcdTxindexDisabled is returned if RawTx is missing from GetBlockVerboseResult,
	// which happens if txindex is not enabled in btcd.
	ErrBtcdTxindexDisabled = errors.New("len(block.RawTx) == 0, make sure txindex is enabled in btcd")
)

const (
	checkHeadDepositPeriod = time.Second * 5
	blockScanPeriod        = time.Second * 5
	depositBufferSize      = 100
)

// Config scanner config info
type Config struct {
	ScanPeriod            time.Duration // scan period in seconds
	DepositBufferSize     int           // size of GetDeposit() channel
	InitialScanHeight     int64         // what blockchain height to begin scanning from
	ConfirmationsRequired int64         // how many confirmations to wait for block
}

// BTCScanner blockchain scanner to check if there're deposit coins
type BTCScanner struct {
	log       logrus.FieldLogger
	btcClient BtcRPCClient
	// Deposit value channel, exposed by public API, intended for public consumption
	base *BaseScanner
	cfg  Config
}

// NewBTCScanner creates scanner instance
func NewBTCScanner(log logrus.FieldLogger, store Storer, btc BtcRPCClient, cfg Config) (*BTCScanner, error) {
	if cfg.ScanPeriod == 0 {
		cfg.ScanPeriod = blockScanPeriod
	}

	if cfg.DepositBufferSize == 0 {
		cfg.DepositBufferSize = depositBufferSize
	}

	bs := NewBaseScanner(store, log.WithField("prefix", "scanner.btc"), cfg)

	return &BTCScanner{
		btcClient: btc,
		log:       log.WithField("prefix", "scanner.btc"),
		base:      bs,
		cfg:       cfg,
	}, nil
}

func (s *BTCScanner) Run() error {
	return s.base.Run(s.getBlockAtHeight, s.GetBlockCount, s.scanBlock, s.waitForNextBlock, s.getBlockHashAndHeight)
}

// Shutdown shutdown the scanner
func (s *BTCScanner) Shutdown() {
	s.log.Info("Closing BTC scanner")
	s.btcClient.Shutdown()
	s.base.Shutdown()
	s.log.Info("Waiting for BTC scanner to stop")
	s.log.Info("BTC scanner stopped")
}

// scanBlock scans for a new BTC block every ScanPeriod.
// When a new block is found, it compares the block against our scanning
// deposit addresses. If a matching deposit is found, it saves it to the DB.
func (s *BTCScanner) scanBlock(blk interface{}) (int, error) {
	block := blk.(*btcjson.GetBlockVerboseResult)
	log := s.log.WithField("hash", block.Hash)
	log = log.WithField("height", block.Height)

	log.Debug("Scanning block")

	store := s.base.GetStore()
	dvs, err := store.ScanBlock(block, CoinTypeBTC)
	if err != nil {
		log.WithError(err).Error("store.ScanBlock failed")
		return 0, err
	}

	log = log.WithField("scannedDeposits", len(dvs))
	log.Infof("Counted %d deposits from block", len(dvs))

	n := 0
	for _, dv := range dvs {
		select {
		case s.base.ScannedDeposits <- dv:
			n++
		case <-s.base.GetQuit():
			return n, errQuit
		}
	}

	return n, nil
}

// getBlockAtHeight returns that block at a specific height
func (s *BTCScanner) getBlockAtHeight(height int64) (interface{}, error) {
	log := s.log.WithField("blockHeight", height)

	hash, err := s.btcClient.GetBlockHash(height)
	if err != nil {
		log.WithError(err).Error("btcClient.GetBlockHash failed")
		return nil, err
	}

	block, err := s.btcClient.GetBlockVerboseTx(hash)
	if err != nil {
		log.WithError(err).Error("btcClient.GetBlockVerboseTx failed")
		return nil, err
	}

	return block, nil
}

// getNextBlock returns the next block from another block, return nil if next block does not exist
func (s *BTCScanner) getNextBlock(block *btcjson.GetBlockVerboseResult) (*btcjson.GetBlockVerboseResult, error) {
	if block.NextHash == "" {
		return nil, errors.New("Block.NextHash is empty")
	}

	nxtHash, err := chainhash.NewHashFromStr(block.NextHash)
	if err != nil {
		s.log.WithError(err).Error("chainhash.NewHashFromStr failed")
		return nil, err
	}

	s.log.WithField("nextHash", nxtHash.String()).Debug("Calling s.btcClient.GetBlockVerboseTx")
	return s.btcClient.GetBlockVerboseTx(nxtHash)
}

func (s *BTCScanner) GetBlockCount() (int64, error) {
	return s.btcClient.GetBlockCount()
}
func (s *BTCScanner) getBlockHashAndHeight(block interface{}) (string, int64) {
	b := block.(*btcjson.GetBlockVerboseResult)
	return b.Hash, b.Height
}

// waitForNextBlock scans for the next block until it is available
func (s *BTCScanner) waitForNextBlock(blk interface{}) (interface{}, error) {
	block := blk.(*btcjson.GetBlockVerboseResult)
	log := s.log.WithField("blockHash", block.Hash)
	log = log.WithField("blockHeight", block.Height)
	log.Debug("Waiting for the next block")

	if block.NextHash == "" {
		log.Info("Block.NextHash is missing, rescanning this block until NextHash is set")

		hash, err := chainhash.NewHashFromStr(block.Hash)
		if err != nil {
			log.WithError(err).Error("chainhash.NewHashFromStr failed")
			return nil, err
		}

		for {
			var err error
			block, err = s.btcClient.GetBlockVerboseTx(hash)
			if err != nil {
				log.WithError(err).Error("btcClient.GetBlockVerboseTx failed, retrying")
			}

			if err != nil || block.NextHash == "" {
				select {
				case <-s.base.GetQuit():
					return nil, errQuit
				case <-time.After(s.base.GetScanPeriod()):
					continue
				}
			}

			break
		}
	}

	for {
		nextBlock, err := s.getNextBlock(block)
		if err != nil {
			log.WithError(err).Error("getNextBlock failed")
		}
		if nextBlock == nil {
			log.Debug("No new block yet")
		}
		if err != nil || nextBlock == nil {
			select {
			case <-s.base.GetQuit():
				return nil, errQuit
			case <-time.After(s.base.GetScanPeriod()):
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
func (s *BTCScanner) AddScanAddress(addr string) error {
	return s.base.GetStore().AddScanAddress(addr, CoinTypeBTC)
}

// GetScanAddresses returns the deposit addresses that need to scan
func (s *BTCScanner) GetScanAddresses() ([]string, error) {
	return s.base.GetStore().GetScanAddresses(CoinTypeBTC)
}

// GetDeposit returns deposit value channel.
func (s *BTCScanner) GetDeposit() <-chan DepositNote {
	return s.base.GetDeposit()
}

func (s *BTCScanner) GetStore() Storer {
	return s.base.GetStore()
}
