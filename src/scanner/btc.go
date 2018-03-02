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
	"github.com/btcsuite/btcutil"
	"github.com/sirupsen/logrus"
)

var (
	errQuit = errors.New("Scanner quit")

	// ErrBtcdTxindexDisabled is returned if RawTx is missing from GetBlockVerboseResult,
	// which happens if txindex is not enabled in btcd.
	ErrBtcdTxindexDisabled = errors.New("len(block.RawTx) == 0, make sure txindex is enabled in btcd")
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
	Base CommonScanner
}

// NewBTCScanner creates scanner instance
func NewBTCScanner(log logrus.FieldLogger, store Storer, btc BtcRPCClient, cfg Config) (*BTCScanner, error) {
	bs := NewBaseScanner(store, log.WithField("prefix", "scanner.btc"), cfg)

	return &BTCScanner{
		btcClient: btc,
		log:       log.WithField("prefix", "scanner.btc"),
		Base:      bs,
	}, nil
}

// Run begins the BTCScanner
func (s *BTCScanner) Run() error {
	return s.Base.Run(s.GetBlockCount, s.getBlockAtHeight, s.waitForNextBlock, s.scanBlock)
}

// Shutdown shutdown the scanner
func (s *BTCScanner) Shutdown() {
	s.log.Info("Closing BTC scanner")
	s.btcClient.Shutdown()
	s.Base.Shutdown()
	s.log.Info("Waiting for BTC scanner to stop")
	s.log.Info("BTC scanner stopped")
}

// scanBlock scans for a new BTC block every ScanPeriod.
// When a new block is found, it compares the block against our scanning
// deposit addresses. If a matching deposit is found, it saves it to the DB.
func (s *BTCScanner) scanBlock(block *CommonBlock) (int, error) {
	log := s.log.WithField("hash", block.Hash)
	log = log.WithField("height", block.Height)

	log.Debug("Scanning block")

	dvs, err := s.Base.GetStorer().ScanBlock(block, CoinTypeBTC)
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

//GetBlockCount returns bitcoin block count
func (s *BTCScanner) GetBlockCount() (int64, error) {
	return s.btcClient.GetBlockCount()
}

// getBlockAtHeight returns that block at a specific height
func (s *BTCScanner) getBlockAtHeight(height int64) (*CommonBlock, error) {
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

	return btcBlock2CommonBlock(block)

}

// btcBlock2CommonBlock convert bitcoin block to common block
func btcBlock2CommonBlock(block *btcjson.GetBlockVerboseResult) (*CommonBlock, error) {
	if len(block.RawTx) == 0 {
		return nil, ErrBtcdTxindexDisabled
	}
	cb := CommonBlock{}
	cb.Hash = block.Hash
	cb.NextHash = block.NextHash
	cb.Height = block.Height
	cb.RawTx = make([]CommonTx, 0, len(block.RawTx))
	for _, tx := range block.RawTx {
		cbTx := CommonTx{}
		cbTx.Txid = tx.Txid
		cbTx.Vout = make([]CommonVout, 0, len(tx.Vout))
		for _, v := range tx.Vout {
			amt, err := btcutil.NewAmount(v.Value)
			if err != nil {
				return nil, err
			}
			cv := CommonVout{}
			cv.Value = int64(amt)
			cv.Addresses = v.ScriptPubKey.Addresses
			cbTx.Vout = append(cbTx.Vout, cv)
		}
		cb.RawTx = append(cb.RawTx, cbTx)
	}

	return &cb, nil
}

// getNextBlock returns the next block from another block, return nil if next block does not exist
func (s *BTCScanner) getNextBlock(block *CommonBlock) (*CommonBlock, error) {
	if block.NextHash == "" {
		return nil, errors.New("Block.NextHash is empty")
	}

	nxtHash, err := chainhash.NewHashFromStr(block.NextHash)
	if err != nil {
		s.log.WithError(err).Error("chainhash.NewHashFromStr failed")
		return nil, err
	}

	s.log.WithField("nextHash", nxtHash.String()).Debug("Calling s.btcClient.GetBlockVerboseTx")
	btc, err := s.btcClient.GetBlockVerboseTx(nxtHash)
	if err != nil {
		s.log.WithError(err).Error("chainhash.NewHashFromStr failed")
		return nil, err
	}
	return btcBlock2CommonBlock(btc)
}

// waitForNextBlock scans for the next block until it is available
func (s *BTCScanner) waitForNextBlock(block *CommonBlock) (*CommonBlock, error) {
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
			btcBlock, err := s.btcClient.GetBlockVerboseTx(hash)
			if err != nil {
				log.WithError(err).Error("btcClient.GetBlockVerboseTx failed, retrying")
			}

			if err != nil || btcBlock.NextHash == "" {
				select {
				case <-s.Base.GetQuitChan():
					return nil, errQuit
				case <-time.After(s.Base.GetScanPeriod()):
					continue
				}
			}
			block, err = btcBlock2CommonBlock(btcBlock)
			if err != nil {
				log.WithError(err).Error("btc block 2 common block failed")
				return nil, err
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
func (s *BTCScanner) AddScanAddress(addr, coinType string) error {
	return s.Base.GetStorer().AddScanAddress(addr, coinType)
}

// GetScanAddresses returns the deposit addresses that need to scan
func (s *BTCScanner) GetScanAddresses() ([]string, error) {
	return s.Base.GetStorer().GetScanAddresses(CoinTypeBTC)
}

//GetDeposit returns channel of depositnote
func (s *BTCScanner) GetDeposit() <-chan DepositNote {
	return s.Base.GetDeposit()
}
