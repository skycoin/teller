package scanner

import (
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	checkHeadDepositPeriod = time.Second * 5
	blockScanPeriod        = time.Second * 5
	depositBufferSize      = 100
)

//BaseScanner common structure that provide the scanning functionality
type BaseScanner struct {
	Cfg      Config
	Store    Storer
	log      logrus.FieldLogger
	DepositC chan DepositNote
	// Internal deposit value channel
	ScannedDeposits chan Deposit
	Quit            chan struct{}
	done            chan struct{}
}

//CommonVout common transaction output info
type CommonVout struct {
	Value     int64
	N         uint32
	Addresses []string
}

//CommonTx common transaction info
type CommonTx struct {
	Txid string
	Vout []CommonVout
}

//CommonBlock interface argument, other coin's block must convert to this type
type CommonBlock struct {
	Height   int64
	Hash     string
	NextHash string
	RawTx    []CommonTx
}

//NewBaseScanner creates base scanner instance
func NewBaseScanner(store Storer, log logrus.FieldLogger, cfg Config) *BaseScanner {
	if cfg.ScanPeriod == 0 {
		cfg.ScanPeriod = blockScanPeriod
	}

	if cfg.DepositBufferSize == 0 {
		cfg.DepositBufferSize = depositBufferSize
	}
	return &BaseScanner{
		log:             log,
		Store:           store,
		Quit:            make(chan struct{}),
		DepositC:        make(chan DepositNote),
		ScannedDeposits: make(chan Deposit, cfg.DepositBufferSize),
		done:            make(chan struct{}),
		Cfg:             cfg,
	}
}

// loadUnprocessedDeposits loads unprocessed Deposits into the scannedDeposits
// channel. This is called during initialization, to resume processing.
func (s *BaseScanner) loadUnprocessedDeposits() error {
	s.log.Info("Loading unprocessed deposit values")

	dvs, err := s.Store.GetUnprocessedDeposits()
	if err != nil {
		s.log.WithError(err).Error("GetUnprocessedDeposits failed")
		return err
	}

	s.log.WithField("depositsLen", len(dvs)).Info("Loaded unprocessed deposit values")

	for _, dv := range dvs {
		select {
		case <-s.Quit:
			return errQuit
		case s.ScannedDeposits <- dv:
		}
	}

	return nil
}

// processDeposit sends a deposit to DepositC, which is read by exchange.Exchange.
// Exchange will reply with an error or nil on the DepositNote's ErrC channel.
// If no error is reported, the deposit will be marked as "processed".
// If this exits early, or the exchange reported an error, the deposit will
// not be marked as processed. When restarted, unprocessed deposits will be
// sent to the exchange for processing again.
func (s *BaseScanner) processDeposit(dv Deposit) error {
	log := s.log.WithField("deposit", dv)
	log.Info("Sending deposit to depositC")

	dn := NewDepositNote(dv)

	select {
	case <-s.Quit:
		return errQuit
	case s.DepositC <- dn:
		select {
		case <-s.Quit:
			return errQuit
		case err, ok := <-dn.ErrC:
			if err == nil {
				if ok {
					if err := s.Store.SetDepositProcessed(dv.ID()); err != nil {
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

//GetScanPeriod returns scan period
func (s *BaseScanner) GetScanPeriod() time.Duration {
	return s.Cfg.ScanPeriod
}

//Shutdown shutdown base scanner
func (s *BaseScanner) Shutdown() {
	close(s.DepositC)
	close(s.Quit)
	<-s.done
}

// Run starts the scanner
func (s *BaseScanner) Run(
	getBlockCount func() (int64, error),
	getBlockAtHeight func(int64) (*CommonBlock, error),
	waitForNextBlock func(*CommonBlock) (*CommonBlock, error),
	scanBlock func(*CommonBlock) (int, error),
) error {
	log := s.log.WithField("config", s.Cfg)
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
	initialBlock, err := getBlockAtHeight(s.Cfg.InitialScanHeight)
	if err != nil {
		log.WithError(err).Error("getBlockAtHeight failed")

		return err
	}

	initHash, initHeight := getBlockHashAndHeight(initialBlock)
	s.log.WithFields(logrus.Fields{
		"initialHash":   initHash,
		"initialHeight": initHeight,
	}).Info("Begin scanning blockchain")

	// This loop scans for a new BTC block every ScanPeriod.
	// When a new block is found, it compares the block against our scanning
	// deposit addresses. If a matching deposit is found, it saves it to the DB.
	log.Info("Launching scan goroutine")
	wg.Add(1)
	go func(block *CommonBlock) {
		defer wg.Done()
		defer log.Info("Scan goroutine exited")

		// Wait before retrying again
		// Returns true if the scanner quit
		wait := func() error {
			select {
			case <-s.Quit:
				return errQuit
			case <-time.After(s.Cfg.ScanPeriod):
				return nil
			}
		}

		deposits := 0
		for {
			select {
			case <-s.Quit:
				return
			default:
			}

			blockHash, blockHeight := getBlockHashAndHeight(block)
			log = log.WithFields(logrus.Fields{
				"height": blockHash,
				"hash":   blockHeight,
			})

			// Check for necessary confirmations
			bestHeight, err := getBlockCount()
			if err != nil {
				log.WithError(err).Error("getBlockCount failed")
				if wait() != nil {
					return
				}

				continue
			}

			log = log.WithField("bestHeight", bestHeight)

			// If not enough confirmations exist for this block, wait
			if blockHeight+s.Cfg.ConfirmationsRequired > bestHeight {
				log.Info("Not enough confirmations, waiting")
				if wait() != nil {
					return
				}
				continue
			}

			// Scan the block for deposits
			n, err := scanBlock(block)
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
			block, err = waitForNextBlock(block)
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
			case <-s.Quit:
				return
			case dv := <-s.ScannedDeposits:
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

func getBlockHashAndHeight(block *CommonBlock) (string, int64) {
	return block.Hash, block.Height
}
