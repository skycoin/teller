// Package scanner scans bitcoin blockchain and check all transactions
// to see if there are addresses in vout that can match our deposit addresses.
// If found, then generate an event and push to deposit event channel
//
// current scanner doesn't support reconnect after btcd shutdown, if
// any error occur when call btcd apis, the scan service will be closed.
package scanner

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/boltdb/bolt"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcutil"
	"github.com/sirupsen/logrus"
)

var (
	errBlockNotFound = errors.New("Block not found")
)

const (
	checkHeadDepositValuePeriod = time.Second * 5
	blockScanPeriod             = time.Second * 5
)

// BTCScanner blockchain scanner to check if there're deposit coins
type BTCScanner struct {
	log       logrus.FieldLogger
	cfg       Config
	btcClient BtcRPCClient
	store     *store
	// Deposit value channel, exposed by public API, intended for public consumption
	depositC chan DepositNote
	// Internal deposit value channel
	scannedDeposits chan Deposit
	quit            chan struct{}
	done            chan struct{}
}

// NewBTCScanner creates scanner instance
func NewBTCScanner(log logrus.FieldLogger, db *bolt.DB, btc BtcRPCClient, cfg Config) (*BTCScanner, error) {
	s, err := newStore(db)
	if err != nil {
		return nil, err
	}

	if cfg.ScanPeriod == 0 {
		cfg.ScanPeriod = blockScanPeriod
	}

	return &BTCScanner{
		btcClient:       btc,
		log:             log.WithField("prefix", "scanner.btc"),
		cfg:             cfg,
		store:           s,
		depositC:        make(chan DepositNote),
		quit:            make(chan struct{}),
		done:            make(chan struct{}),
		scannedDeposits: make(chan Deposit, 100),
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

	// get last scan block
	lsb, err := s.store.getLastScanBlock()
	if err != nil {
		return fmt.Errorf("Get last scan block failed: %v", err)
	}

	height := lsb.Height
	hash := lsb.Hash

	log = log.WithFields(logrus.Fields{
		"blockHeight": height,
		"blockHash":   hash,
	})

	// load unprocessed deposits
	if err := s.loadUnprocessedDeposits(); err != nil {
		return err
	}

	if height == 0 {
		// the first time the bot start
		// get the best block
		block, err := s.getBestBlock()
		if err != nil {
			return err
		}

		if err := s.scanBlock(block); err != nil {
			return fmt.Errorf("Scan block %s failed: %v", block.Hash, err)
		}

		hash = block.Hash
		log = log.WithField("blockHash", hash)
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

			nextBlock, err := s.getNextBlock(hash)
			if err != nil {
				log.WithError(err).Error("getNextBlock failed")
			}

			if nextBlock == nil {
				log.Debug("No new block to scan...")

			}

			if err != nil || nextBlock == nil {
				select {
				case <-s.quit:
					return
				case <-time.After(s.cfg.ScanPeriod):
					continue
				}
			}

			hash = nextBlock.Hash
			height = nextBlock.Height
			log = log.WithFields(logrus.Fields{
				"blockHeight": height,
				"blockHash":   hash,
			})

			log.Debug("Scanned new block")

			if err := s.scanBlock(nextBlock); err != nil {
				log.WithError(err).Error("Scan block failed")

				// Wait before retrying again
				select {
				case <-s.quit:
					return
				case <-time.After(s.cfg.ScanPeriod):
					continue
				}
			}
		}
	}(hash, height)

	// This loop gets the head deposit value (from an array saved in the db)
	// It sends each head to depositC, which is processed by Exchange.
	// The loop blocks until the Exchange writes to the ErrC channel
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			// TODO: Review the error handling down the stack through
			// streamDepositValues. It may be a problem to return early on error,
			// and this may need reprocessing logic.
			if err := s.streamDepositValues(); err != nil {
				log.WithError(err).Error("streamDepositValues error")
			}
			select {
			case <-time.After(checkHeadDepositValuePeriod):
				continue
			case <-s.quit:
				return
			}
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	<-done

	return nil
}

// Shutdown shutdown the scanner
func (s *BTCScanner) Shutdown() {
	s.log.Info("Closing BTC scanner")
	close(s.quit)
	s.btcClient.Shutdown()
	s.log.Info("Waiting for BTC scanner to stop")
	<-s.done
	s.log.Info("BTC scanner stopped")
}

// loadUnprocessedDeposits loads unprocessed DepositValues into the scannedDeposits
// channel. This is called during initialization, to resume processing.
func (s *BTCScanner) loadUnprocessedDeposits() error {
	s.log.Info("Loading unprocessed deposit values")

	dvs, err := s.store.getUnprocessedDepositValues()
	if err != nil {
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

func (s *BTCScanner) streamDepositValues() error {
	for {
		select {
		case <-s.quit:
			return nil
		case dv := <-s.scannedDeposits:
			if err := s.processDepositValue(dv); err != nil {
				s.log.WithField("deposit", dv).WithError(err).Error("processDepositValue failed")
				return err
			}
		}
	}
}

func (s *BTCScanner) processDepositValue(dv Deposit) error {
	log := s.log.WithField("deposit", dv)
	log.Info("Sending deposit to depositC")

	dn := makeDepositNote(dv)

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
					if err := s.store.setDepositValueProcessed(dv.TxN()); err != nil {
						log.WithError(err).Error("setDepositValueProcessed error")
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

func (s *BTCScanner) scanBlock(block *btcjson.GetBlockVerboseResult) error {
	var dvs []Deposit

	if err := s.store.db.Update(func(tx *bolt.Tx) error {
		addrs, err := s.store.getScanAddressesTx(tx)
		if err != nil {
			return err
		}

		dvs, err = scanBTCBlock(block, addrs)
		if err != nil {
			return err
		}

		for _, dv := range dvs {
			if err := s.store.pushDepositValueTx(tx, dv); err != nil {
				log := s.log.WithField("deposit", dv)
				switch err.(type) {
				case DepositValueExistsErr:
					log.Warning("Deposit already exists in db")
					continue
				default:
					log.WithError(err).Error("pushDepositValueTx failed")
					return err
				}
			}
		}

		hash, err := chainhash.NewHashFromStr(block.Hash)
		if err != nil {
			return err
		}

		return s.store.setLastScanBlockTx(tx, LastScanBlock{
			Hash:   hash.String(),
			Height: block.Height,
		})
	}); err != nil {
		return err
	}

	for _, dv := range dvs {
		select {
		case s.scannedDeposits <- dv:
		case <-s.quit:
			return nil
		}
	}

	return nil
}

// scanBTCBlock scan the given block and returns the next block hash or error
func scanBTCBlock(block *btcjson.GetBlockVerboseResult, depositAddrs []string) ([]Deposit, error) {
	addrMap := map[string]struct{}{}
	for _, a := range depositAddrs {
		addrMap[a] = struct{}{}
	}

	var dv []Deposit
	for _, tx := range block.RawTx {
		for _, v := range tx.Vout {
			amt, err := btcutil.NewAmount(v.Value)
			if err != nil {
				return nil, err
			}

			for _, a := range v.ScriptPubKey.Addresses {
				if _, ok := addrMap[a]; ok {
					dv = append(dv, Deposit{
						Address: a,
						Value:   int64(amt),
						Height:  block.Height,
						Tx:      tx.Txid,
						N:       v.N,
					})
				}
			}
		}
	}

	return dv, nil
}

// AddScanAddress adds new scan address
func (s *BTCScanner) AddScanAddress(addr string) error {
	return s.store.addScanAddress(addr)
}

// GetBestBlock returns the hash and height of the block in the longest (best)
// chain.
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
	return s.store.getScanAddresses()
}

// GetDeposit returns deposit value channel.
func (s *BTCScanner) GetDeposit() <-chan DepositNote {
	return s.depositC
}
