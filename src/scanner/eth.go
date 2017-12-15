// Package scanner scans ethcoin blockchain and check all transactions
// to see if there are addresses in vout that can match our deposit addresses.
// If found, then generate an event and push to deposit event channel
//
// current scanner doesn't support reconnect after btcd shutdown, if
// any error occur when call btcd apis, the scan service will be closed.
package scanner

import (
	"context"
	"math/big"
	"strconv"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/sirupsen/logrus"
)

// ETHScanner blockchain scanner to check if there're deposit coins
type ETHScanner struct {
	log       logrus.FieldLogger
	cfg       Config
	ethClient EthRPCClient
	store     Storer
	// Deposit value channel, exposed by public API, intended for public consumption
	depositC chan DepositNote
	// Internal deposit value channel
	scannedDeposits chan Deposit
	quit            chan struct{}
	done            chan struct{}
}

// NewETHScanner creates scanner instance
func NewETHScanner(log logrus.FieldLogger, store Storer, eth EthRPCClient, cfg Config) (*ETHScanner, error) {
	if cfg.ScanPeriod == 0 {
		cfg.ScanPeriod = blockScanPeriod
	}

	if cfg.DepositBufferSize == 0 {
		cfg.DepositBufferSize = depositBufferSize
	}

	return &ETHScanner{
		ethClient:       eth,
		log:             log.WithField("prefix", "scanner.eth"),
		cfg:             cfg,
		store:           store,
		depositC:        make(chan DepositNote),
		quit:            make(chan struct{}),
		done:            make(chan struct{}),
		scannedDeposits: make(chan Deposit, depositBufferSize),
	}, nil
}

// Run starts the scanner
func (s *ETHScanner) Run() error {
	log := s.log.WithField("config", s.cfg)
	log.Info("Start ethcoin blockchain scan service")
	defer func() {
		log.Info("Ethcoin blockchain scan service closed")
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
	initialBlock, err := s.getBlockAtHeight(uint64(s.cfg.InitialScanHeight))
	if err != nil {
		log.WithError(err).Error("getBlockAtHeight failed")

		// If teller is shutdown while this call is in progress, the rpcclient
		// returns ErrClientShutdown. This is an expected condition and not
		// an error, so return nil
		//if err == rpcclient.ErrClientShutdown {
		//return nil
		//}
		return err
	}

	s.log.WithFields(logrus.Fields{
		"initialHash":   initialBlock.Hash().String(),
		"initialHeight": initialBlock.NumberU64(),
	}).Info("Begin scanning blockchain")

	// This loop scans for a new ETH block every ScanPeriod.
	// When a new block is found, it compares the block against our scanning
	// deposit addresses. If a matching deposit is found, it saves it to the DB.
	log.Info("Launching scan goroutine")
	wg.Add(1)
	go func(block *types.Block) {
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
				"height": block.NumberU64(),
				"hash":   block.Hash().String(),
			})

			// Check for necessary confirmations
			bestHeight, err := s.ethClient.GetBlockCount()
			if err != nil {
				log.WithError(err).Error("ethClient.GetBlockCount failed")
				if wait() != nil {
					return
				}

				continue
			}

			log = log.WithField("bestHeight", bestHeight)

			// If not enough confirmations exist for this block, wait
			if int64(block.NumberU64())+s.cfg.ConfirmationsRequired > bestHeight {
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
func (s *ETHScanner) Shutdown() {
	s.log.Info("Closing ETH scanner")
	close(s.quit)
	close(s.depositC)
	s.ethClient.Shutdown()
	s.log.Info("Waiting for ETH scanner to stop")
	<-s.done
	s.log.Info("ETH scanner stopped")
}

// loadUnprocessedDeposits loads unprocessed Deposits into the scannedDeposits
// channel. This is called during initialization, to resume processing.
func (s *ETHScanner) loadUnprocessedDeposits() error {
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
func (s *ETHScanner) processDeposit(dv Deposit) error {
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

// scanBlock scans for a new ETH block every ScanPeriod.
// When a new block is found, it compares the block against our scanning
// deposit addresses. If a matching deposit is found, it saves it to the DB.
func (s *ETHScanner) scanBlock(block *types.Block) (int, error) {
	log := s.log.WithField("hash", block.Hash().String())
	log = log.WithField("height", block.NumberU64())

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
func (s *ETHScanner) getBlockAtHeight(seq uint64) (*types.Block, error) {
	return s.ethClient.GetBlockVerboseTx(seq)
}

// getNextBlock returns the next block of given hash, return nil if next block does not exist
func (s *ETHScanner) getNextBlock(seq uint64) (*types.Block, error) {
	return s.ethClient.GetBlockVerboseTx(seq + 1)
}

// waitForNextBlock scans for the next block until it is available
func (s *ETHScanner) waitForNextBlock(block *types.Block) (*types.Block, error) {
	log := s.log.WithField("blockHash", block.Hash().String())
	log = log.WithField("blockHeight", block.NumberU64())
	log.Debug("Waiting for the next block")

	for {
		nextBlock, err := s.getNextBlock(block.NumberU64())
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
			"hash":   nextBlock.Hash().String(),
			"height": nextBlock.NumberU64(),
		}).Debug("Found nextBlock")

		return nextBlock, nil
	}
}

// AddScanAddress adds new scan address
func (s *ETHScanner) AddScanAddress(addr string) error {
	return s.store.AddScanAddress(addr, CoinTypeETH)
}

// GetScanAddresses returns the deposit addresses that need to scan
func (s *ETHScanner) GetScanAddresses() ([]string, error) {
	return s.store.GetScanAddresses(CoinTypeETH)
}

// GetDeposit returns deposit value channel.
func (s *ETHScanner) GetDeposit() <-chan DepositNote {
	return s.depositC
}

//EthClient is self-defined struct for implement EthRPCClient interface
//because origin rpc.Client has't required interface
type EthClient struct {
	c *rpc.Client
}

//NewEthClient create ethereum rpc client
func NewEthClient(server, port string) (*EthClient, error) {
	ethrpc, err := rpc.Dial("http://" + server + ":" + port)
	if err != nil {
		return nil, err
	}
	ec := EthClient{c: ethrpc}
	return &ec, nil
}

//GetBlockCount returns ethereum block count
func (ec *EthClient) GetBlockCount() (int64, error) {
	var bn string
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := ec.c.CallContext(ctx, &bn, "eth_blockNumber"); err != nil {
		return 0, err
	}
	bnRealStr := bn[2:]
	blockNum, err := strconv.ParseInt(bnRealStr, 16, 32)
	if err != nil {
		return 0, err
	}
	return int64(blockNum), nil
}

//Shutdown close rpc connection
func (ec *EthClient) Shutdown() {
	ec.c.Close()
}

//GetBlockVerboseTx returns ethereum block data
func (ec *EthClient) GetBlockVerboseTx(seq uint64) (*types.Block, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	block, err := ethclient.NewClient(ec.c).BlockByNumber(ctx, big.NewInt(int64(seq)))
	if err != nil && err.Error() != "not found" {
		return nil, err
	}

	return block, nil
}

//GetTransaction returns transaction by txhash
func (ec *EthClient) GetTransaction(txhash common.Hash) (*types.Transaction, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	//txhash := common.StringToHash(txid)
	tx, _, err := ethclient.NewClient(ec.c).TransactionByHash(ctx, txhash)
	if err != nil {
		return nil, err
	}
	return tx, nil
}
