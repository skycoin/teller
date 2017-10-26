// Package scanner scans bitcoin blockchain and check all transactions
// to see if there are addresses in vout that can match our deposit addresses.
// If found, then generate an event and push to deposit event channel
//
// current scanner doesn't support reconnect after btcd shutdown, if
// any error occur when call btcd apis, the scan service will be closed.
package scanner

import (
	"errors"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/sirupsen/logrus"
)

var (
	errBlockNotFound = errors.New("Block not found")
	errNoNewBlock    = errors.New("No new block")
)

const (
	checkHeadDepositPeriod = time.Second * 5
	blockScanPeriod        = time.Second * 5
)

// Config scanner config info
type Config struct {
	ScanPeriod        time.Duration // scan period in seconds
	DepositBufferSize int           // size of GetDeposit() channel
}

// BTCScanner blockchain scanner to check if there're deposit coins
type BTCScanner struct {
	log       logrus.FieldLogger
	cfg       Config
	btcClient BtcRPCClient
	store     Storer
	// Deposit value channel, exposed by public API, intended for public consumption
	depositC chan DepositNote
	// Internal deposit value channel
	scannedDeposits chan Deposit
	quit            chan struct{}
	done            chan struct{}
}

// NewBTCScanner creates scanner instance
func NewBTCScanner(log logrus.FieldLogger, store Storer, btc BtcRPCClient, cfg Config) (*BTCScanner, error) {
	if cfg.ScanPeriod == 0 {
		cfg.ScanPeriod = blockScanPeriod
	}

	if cfg.DepositBufferSize == 0 {
		cfg.DepositBufferSize = 100
	}

	return &BTCScanner{
		btcClient:       btc,
		log:             log.WithField("prefix", "scanner.btc"),
		cfg:             cfg,
		store:           store,
		depositC:        make(chan DepositNote),
		quit:            make(chan struct{}),
		done:            make(chan struct{}),
		scannedDeposits: make(chan Deposit, cfg.DepositBufferSize),
	}, nil
}

// Run starts the scanner
func (s *BTCScanner) Run() error {
	log := s.log
	log.Info("Start bitcoin blockchain scan service")
	defer func() {
		log.Info("Bitcoin blockchain scan service closed")
		close(s.done)
	}()

	var wg sync.WaitGroup

	// Load unprocessed deposits
	if err := s.loadUnprocessedDeposits(); err != nil {
		log.WithError(err).Error("loadUnprocessedDeposits failed")
		return err
	}

	// Get the last scanned block
	lsb, err := s.store.GetLastScanBlock()
	if err != nil {
		log.WithError(err).Error("GetLastScanBlock failed")
		return err
	}

	height := lsb.Height
	hash := lsb.Hash

	log.WithFields(logrus.Fields{
		"blockHeight": height,
		"blockHash":   hash,
	}).Info("Last scanned block found")

	// NOTE: This means that it starts scanning from the best block at the time
	// the scanner database is initialized.
	// TODO -- the "start" block height should be configurable, we begin
	// scanning from there.
	if height == 0 {
		// The first time the bot starts, get the best block
		block, err := s.getBestBlock()
		if err != nil {
			return err
		}

		log = log.WithField("blockHash", block.Hash)

		nextBlock, err := s.scanBlock(block.Hash, block.Height)
		if err != nil {
			log.WithError(err).Error("scanBlock failed")
			return err
		}

		// nextBlock is nil with no error if the scanner quit
		if nextBlock == nil {
			return nil
		}

		hash = nextBlock.Hash
		height = nextBlock.Height
	}

	// This loop scans for a new BTC block every ScanPeriod.
	// When a new block is found, it compares the block against our scanning
	// deposit addresses. If a matching deposit is found, it saves it to the DB.
	wg.Add(1)
	go func(hash string, height int64) {
		defer wg.Done()

		for {
			select {
			case <-s.quit:
				return
			default:
			}

			nextBlock, err := s.scanBlock(hash, height)
			if err != nil {
				switch err {
				case errNoNewBlock:
				default:
					log.WithError(err).Error("Scan block failed")
				}

				// Wait before retrying again
				log.WithField("scanPeriod", s.cfg.ScanPeriod).Debug("Waiting to scan blocks again")
				select {
				case <-s.quit:
					return
				case <-time.After(s.cfg.ScanPeriod):
					continue
				}
			}

			// nextBlock is nil with no error if the scanner quit
			if nextBlock == nil {
				return
			}

			hash = nextBlock.Hash
			height = nextBlock.Height
		}
	}(hash, height)

	// This loop gets the head deposit value (from an array saved in the db)
	// It sends each head to depositC, which is processed by Exchange.
	// The loop blocks until the Exchange writes to the ErrC channel
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-s.quit:
				return
			case dv := <-s.scannedDeposits:
				if err := s.processDeposit(dv); err != nil {
					msg := "processDeposit failed. This deposit will be reprocessed the next time the scanner is run."
					s.log.WithField("deposit", dv).WithError(err).Error(msg)
				}
			}
		}
	}()

	wg.Wait()

	return nil
}

// Shutdown shutdown the scanner
func (s *BTCScanner) Shutdown() {
	s.log.Info("Closing BTC scanner")
	close(s.depositC)
	close(s.quit)
	s.btcClient.Shutdown()
	s.log.Info("Waiting for BTC scanner to stop")
	<-s.done
	s.log.Info("BTC scanner stopped")
}

// loadUnprocessedDeposits loads unprocessed Deposits into the scannedDeposits
// channel. This is called during initialization, to resume processing.
func (s *BTCScanner) loadUnprocessedDeposits() error {
	s.log.Info("Loading unprocessed deposit values")

	dvs, err := s.store.GetUnprocessedDeposits()
	if err != nil {
		s.log.WithError(err).Error("GetUnprocessedDeposits failed")
		return err
	}

	s.log.WithField("depositsLen", len(dvs)).Info("Loaded unprocessed deposit values")

	for _, dv := range dvs {
		select {
		case <-s.quit:
			return nil
		case s.scannedDeposits <- dv:
		}
	}

	return nil
}

// processDeposit sends a deposit to depositC, which is read by exchange.Exchange.
// Exchange will reply with an error or nil on the DepositNote's ErrC channel.
// If no error is reported, the deposit will be marked as "processed".
// If this exits early, or the exchange reported an error, the deposit will
// not be marked as processed. When restarted, unprocessed deposits will be
// sent to the exchange for processing again.
func (s *BTCScanner) processDeposit(dv Deposit) error {
	log := s.log.WithField("deposit", dv)
	log.Info("Sending deposit to depositC")

	dn := NewDepositNote(dv)

	select {
	case <-s.quit:
		return nil
	case s.depositC <- dn:
		select {
		case <-s.quit:
			return nil
		case err, ok := <-dn.ErrC:
			if err == nil {
				if ok {
					if err := s.store.SetDepositProcessed(dv.TxN()); err != nil {
						log.WithError(err).Error("SetDepositProcessed error")
						return err
					}
					log.Info("Deposit is processed")
				} else {
					log.Warn("DepositNote.ErrC unexpectedly closed")
				}
			} else {
				log.WithError(err).Error("DepositNote.ErrC error")
				return err
			}
		}
	}

	return nil
}

// scanBlock scans for a new BTC block every ScanPeriod.
// When a new block is found, it compares the block against our scanning
// deposit addresses. If a matching deposit is found, it saves it to the DB.
func (s *BTCScanner) scanBlock(hash string, height int64) (*btcjson.GetBlockVerboseResult, error) {
	log := s.log.WithField("hash", hash)
	log = log.WithField("height", height)

	nextBlock, err := s.getNextBlock(hash)
	if err != nil {
		log.WithError(err).Error("getNextBlock failed")
		return nil, err
	}

	if nextBlock == nil {
		log.Debug("No new block to scan...")
		return nil, errNoNewBlock
	}

	log = log.WithFields(logrus.Fields{
		"nextHeight": nextBlock.Height,
		"nextHash":   nextBlock.Hash,
	})

	log.Debug("Scanned new block")

	dvs, err := s.store.ScanBlock(nextBlock)
	if err != nil {
		s.log.WithError(err).Error("store.ScanBlock failed")
		return nil, err
	}

	for _, dv := range dvs {
		select {
		case s.scannedDeposits <- dv:
		case <-s.quit:
			return nil, nil
		}
	}

	return nextBlock, nil
}

// AddScanAddress adds new scan address
func (s *BTCScanner) AddScanAddress(addr string) error {
	return s.store.AddScanAddress(addr)
}

// GetBestBlock returns the hash and height of the block in the longest (best) chain.
func (s *BTCScanner) getBestBlock() (*btcjson.GetBlockVerboseResult, error) {
	hash, _, err := s.btcClient.GetBestBlock()
	if err != nil {
		return nil, err
	}

	return s.getBlock(hash)
}

// getBlock returns block of given hash
func (s *BTCScanner) getBlock(hash *chainhash.Hash) (*btcjson.GetBlockVerboseResult, error) {
	return s.btcClient.GetBlockVerboseTx(hash)
}

// getNextBlock returns the next block of given hash, return nil if next block does not exist
func (s *BTCScanner) getNextBlock(hash string) (*btcjson.GetBlockVerboseResult, error) {
	h, err := chainhash.NewHashFromStr(hash)
	if err != nil {
		return nil, err
	}

	b, err := s.getBlock(h)
	if err != nil {
		return nil, err
	}

	if b.NextHash == "" {
		return nil, nil
	}

	nxtHash, err := chainhash.NewHashFromStr(b.NextHash)
	if err != nil {
		return nil, err
	}

	return s.getBlock(nxtHash)
}

// GetScanAddresses returns the deposit addresses that need to scan
func (s *BTCScanner) GetScanAddresses() ([]string, error) {
	return s.store.GetScanAddresses()
}

// GetDeposit returns deposit value channel.
func (s *BTCScanner) GetDeposit() <-chan DepositNote {
	return s.depositC
}
