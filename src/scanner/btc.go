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
	"github.com/btcsuite/btcd/rpcclient"
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
		cfg.DepositBufferSize = depositBufferSize
	}

	return &BTCScanner{
		btcClient:       btc,
		log:             log.WithField("prefix", "scanner.btc"),
		cfg:             cfg,
		store:           store,
		depositC:        make(chan DepositNote),
		quit:            make(chan struct{}),
		done:            make(chan struct{}),
		scannedDeposits: make(chan Deposit, depositBufferSize),
	}, nil
}

// Run starts the scanner
func (s *BTCScanner) Run() error {
	log := s.log.WithField("config", s.cfg)
	log.Info("Start bitcoin blockchain scan service")
	defer func() {
		log.Info("Bitcoin blockchain scan service closed")
		close(s.done)
	}()

	var wg sync.WaitGroup

	// Load unprocessed deposits
	log.Info("Loading unprocessed deposits")
	if err := s.loadUnprocessedDeposits(); err != nil {
		if err == errQuit {
			return nil
		}

		log.WithError(err).Error("loadUnprocessedDeposits failed")
		return err
	}

	// Load the initial scan block
	log.Info("Loading the initial scan block")
	initialBlock, err := s.getBlockAtHeight(s.cfg.InitialScanHeight)
	if err != nil {
		log.WithError(err).Error("getBlockAtHeight failed")

		// If teller is shutdown while this call is in progress, the rpcclient
		// returns ErrClientShutdown. This is an expected condition and not
		// an error, so return nil
		if err == rpcclient.ErrClientShutdown {
			return nil
		}
		return err
	}

	if len(initialBlock.RawTx) == 0 {
		err := ErrBtcdTxindexDisabled
		log.WithError(err).Error("Txindex looks disabled, aborting")
		return err
	}

	s.log.WithFields(logrus.Fields{
		"initialHash":   initialBlock.Hash,
		"initialHeight": initialBlock.Height,
	}).Info("Begin scanning blockchain")

	// This loop scans for a new BTC block every ScanPeriod.
	// When a new block is found, it compares the block against our scanning
	// deposit addresses. If a matching deposit is found, it saves it to the DB.
	log.Info("Launching scan goroutine")
	wg.Add(1)
	go func(block *btcjson.GetBlockVerboseResult) {
		defer wg.Done()
		defer log.Info("Scan goroutine exited")

		// Wait before retrying again
		// Returns true if the scanner quit
		wait := func() error {
			select {
			case <-s.quit:
				return errQuit
			case <-time.After(s.cfg.ScanPeriod):
				return nil
			}
		}

		deposits := 0
		for {
			select {
			case <-s.quit:
				return
			default:
			}

			log = log.WithFields(logrus.Fields{
				"height": block.Height,
				"hash":   block.Hash,
			})

			// Check for necessary confirmations
			bestHeight, err := s.btcClient.GetBlockCount()
			if err != nil {
				log.WithError(err).Error("btcClient.GetBlockCount failed")
				if wait() != nil {
					return
				}

				continue
			}

			log = log.WithField("bestHeight", bestHeight)

			// If not enough confirmations exist for this block, wait
			if block.Height+s.cfg.ConfirmationsRequired > bestHeight {
				log.Info("Not enough confirmations, waiting")
				if wait() != nil {
					return
				}

				continue
			}

			// Scan the block for deposits
			n, err := s.scanBlock(block)
			if err != nil {
				if err == errQuit {
					return
				}

				log.WithError(err).Error("Scan block failed")
				if wait() != nil {
					return
				}

				continue
			}

			deposits += n
			log.WithFields(logrus.Fields{
				"scannedDeposits":      n,
				"totalScannedDeposits": deposits,
			}).Infof("Scanned %d deposits from block", n)

			// Wait for the next block
			block, err = s.waitForNextBlock(block)
			if err != nil {
				if err == errQuit {
					return
				}

				log.WithError(err).Error("s.waitForNextBlock failed")
				if wait() != nil {
					return
				}
				continue
			}
		}
	}(initialBlock)

	// This loop gets the head deposit value (from an array saved in the db)
	// It sends each head to depositC, which is processed by Exchange.
	// The loop blocks until the Exchange writes to the ErrC channel
	log.Info("Launching deposit pipe goroutine")
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer log.Info("Deposit pipe goroutine exited")
		for {
			select {
			case <-s.quit:
				return
			case dv := <-s.scannedDeposits:
				if err := s.processDeposit(dv); err != nil {
					if err == errQuit {
						return
					}

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
	close(s.quit)
	close(s.depositC)
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
			return errQuit
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
		return errQuit
	case s.depositC <- dn:
		select {
		case <-s.quit:
			return errQuit
		case err, ok := <-dn.ErrC:
			if err == nil {
				if ok {
					if err := s.store.SetDepositProcessed(dv.ID()); err != nil {
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
func (s *BTCScanner) scanBlock(block *btcjson.GetBlockVerboseResult) (int, error) {
	log := s.log.WithField("hash", block.Hash)
	log = log.WithField("height", block.Height)

	log.Debug("Scanning block")

	dvs, err := s.store.ScanBlock(block)
	if err != nil {
		log.WithError(err).Error("store.ScanBlock failed")
		return 0, err
	}

	log = log.WithField("scannedDeposits", len(dvs))
	log.Infof("Counted %d deposits from block", len(dvs))

	n := 0
	for _, dv := range dvs {
		select {
		case s.scannedDeposits <- dv:
			n++
		case <-s.quit:
			return n, errQuit
		}
	}

	return n, nil
}

// getBlockAtHeight returns that block at a specific height
func (s *BTCScanner) getBlockAtHeight(height int64) (*btcjson.GetBlockVerboseResult, error) {
	log := s.log.WithField("blockHeight", height)

	hash, err := s.btcClient.GetBlockHash(s.cfg.InitialScanHeight)
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

// waitForNextBlock scans for the next block until it is available
func (s *BTCScanner) waitForNextBlock(block *btcjson.GetBlockVerboseResult) (*btcjson.GetBlockVerboseResult, error) {
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
				case <-s.quit:
					return nil, errQuit
				case <-time.After(s.cfg.ScanPeriod):
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
			case <-s.quit:
				return nil, errQuit
			case <-time.After(s.cfg.ScanPeriod):
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
	return s.store.AddScanAddress(addr)
}

// GetScanAddresses returns the deposit addresses that need to scan
func (s *BTCScanner) GetScanAddresses() ([]string, error) {
	return s.store.GetScanAddresses()
}

// GetDeposit returns deposit value channel.
func (s *BTCScanner) GetDeposit() <-chan DepositNote {
	return s.depositC
}
