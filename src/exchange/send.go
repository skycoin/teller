package exchange

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/skycoin/skycoin/src/api/cli"
	"github.com/skycoin/skycoin/src/coin"
	"github.com/skycoin/skycoin/src/util/droplet"

	"github.com/skycoin/teller/src/config"
	"github.com/skycoin/teller/src/scanner"
	"github.com/skycoin/teller/src/sender"
	"github.com/skycoin/teller/src/util/mathutil"
)

// Sender is a component for sending coins
type Sender interface {
	Status() error
	Balance() (*cli.Balance, error)
}

// SendRunner a Sender than can be run
type SendRunner interface {
	Runner
	Sender
}

// Send reads deposits from a Processor and sends coins
type Send struct {
	log         logrus.FieldLogger
	cfg         config.SkyExchanger
	processor   Processor
	sender      sender.Sender // sender provides APIs for sending skycoin
	store       Storer        // deposit info storage
	quit        chan struct{}
	done        chan struct{}
	depositChan chan DepositInfo
	statusLock  sync.RWMutex
	status      error
}

// NewSend creates exchange service
func NewSend(log logrus.FieldLogger, cfg config.SkyExchanger, store Storer, sender sender.Sender, processor Processor) (*Send, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	if cfg.TxConfirmationCheckWait == 0 {
		cfg.TxConfirmationCheckWait = txConfirmationCheckWait
	}

	return &Send{
		cfg:         cfg,
		log:         log.WithField("prefix", "teller.exchange.send"),
		processor:   processor,
		sender:      sender,
		store:       store,
		quit:        make(chan struct{}),
		done:        make(chan struct{}, 1),
		depositChan: make(chan DepositInfo, 100),
	}, nil
}

// Run starts the exchange process
func (s *Send) Run() error {
	log := s.log
	log.Info("Start exchange service...")
	defer func() {
		log.Info("Closed exchange service")
		s.done <- struct{}{}
	}()

	var wg sync.WaitGroup

	if s.cfg.SendEnabled {
		// Load StatusWaitSend deposits for processing later
		waitSendDeposits, err := s.store.GetDepositInfoArray(func(di DepositInfo) bool {
			return di.Status == StatusWaitSend
		})

		if err != nil {
			err = fmt.Errorf("GetDepositInfoArray failed: %v", err)
			log.WithError(err).Error(err)
			return err
		}

		// Load StatusWaitConfirm deposits for processing later
		waitConfirmDeposits, err := s.store.GetDepositInfoArray(func(di DepositInfo) bool {
			return di.Status == StatusWaitConfirm
		})

		if err != nil {
			err = fmt.Errorf("GetDepositInfoArray failed: %v", err)
			log.WithError(err).Error(err)
			return err
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			s.runSend()
		}()

		// Queue the saved StatusWaitConfirm deposits
		for _, di := range waitConfirmDeposits {
			s.depositChan <- di
		}

		// Queue the saved StatusWaitSend deposits
		for _, di := range waitSendDeposits {
			s.depositChan <- di
		}
	} else {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.runNoSend()
		}()
	}

	// Merge processor.Deposits() into the internal depositChan
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.receiveDeposits()
	}()

	wg.Wait()

	return nil
}

func (s *Send) runSend() {
	// This loop processes StatusWaitSend deposits.
	// Only one deposit is processed at a time; it will not send more coins
	// until it receives confirmation of the previous send.
	log := s.log.WithField("goroutine", "runSend")
	for {
		select {
		case <-s.quit:
			log.Info("quit")
			return
		case d := <-s.depositChan:
			log := log.WithField("depositInfo", d)
			if err := s.processWaitSendDeposit(d); err != nil {
				log.WithError(err).Error("processWaitSendDeposit failed. This deposit will not be reprocessed until teller is restarted.")
			}
		}
	}
}

func (s *Send) runNoSend() {
	// Flush the deposit channel so that it doesn't fill up
	log := s.log.WithField("goroutine", "runNoSend")
	for {
		select {
		case <-s.quit:
			log.Info("quit")
			return
		case d := <-s.depositChan:
			log := log.WithField("depositInfo", d)
			log.Warning("Received depositInfo, but sending is disabled")
		}
	}
}

func (s *Send) receiveDeposits() {
	// Read deposits from the processor and place them on the internal deposit channel
	// This is necessary as a separate step because deposits can come from other sources,
	// specifically deposits that were partially in processing from a previous run will
	// be loaded first, and not come from the receiver.
	log := s.log.WithField("goroutine", "receiveDeposits")
	for {
		select {
		case <-s.quit:
			log.Info("quit")
			return
		case d := <-s.processor.Deposits():
			log.WithField("depositInfo", d).Info("Received deposit from processor")
			s.depositChan <- d
		}
	}
}

// Shutdown close the exchange service
func (s *Send) Shutdown() {
	close(s.quit)
	s.log.Info("Waiting for Run() to finish")
	<-s.done
	s.log.Info("Shutdown complete")
}

// processDeposit advances a single deposit through three states:
// StatusWaitSend -> StatusWaitConfirm
// StatusWaitConfirm -> StatusDone
// StatusWaitDeposit is never saved to the database, so it does not transition
func (s *Send) processWaitSendDeposit(di DepositInfo) error {
	log := s.log.WithField("depositInfo", di)
	log.Info("Processing StatusWaitSend deposit")

	for {
		select {
		case <-s.quit:
			return nil
		default:
		}

		log.Info("handleDepositInfoState")

		var err error
		di, err = s.handleDepositInfoState(di)
		log = log.WithField("depositInfo", di)

		s.setStatus(err)

		switch err.(type) {
		case sender.RPCError:
			// Treat skycoin RPC/CLI errors as temporary.
			// Some RPC/CLI errors are hypothetically permanent,
			// but most likely it is an insufficient wallet balance or
			// the skycoin node is unavailable.
			// A permanent error suggests a bug in skycoin or teller so can be fixed.
			log.WithError(err).Error("handleDepositInfoState failed")
			select {
			case <-time.After(s.cfg.TxConfirmationCheckWait):
			case <-s.quit:
				return nil
			}
		default:
			switch err {
			case nil:
				break
			case ErrNotConfirmed:
				select {
				case <-time.After(s.cfg.TxConfirmationCheckWait):
				case <-s.quit:
					return nil
				}
			default:
				log.WithError(err).Error("handleDepositInfoState failed")
				return err
			}
		}

		if di.Status == StatusDone {
			return nil
		}
	}
}

func (s *Send) handleDepositInfoState(di DepositInfo) (DepositInfo, error) {
	log := s.log.WithField("depositInfo", di)

	if err := di.ValidateForStatus(); err != nil {
		log.WithError(err).Error("handleDepositInfoState's DepositInfo is invalid")
		return di, err
	}

	switch di.Status {
	case StatusWaitSend:
		// Prepare skycoin transaction
		skyTx, err := s.createTransaction(di)

		if err != nil {
			log.WithError(err).Error("createTransaction failed")

			// If the send amount is empty, skip to StatusDone.
			if err == ErrEmptySendAmount {
				log.Info("Send amount is 0, skipping to StatusDone")
				di, err = s.store.UpdateDepositInfo(di.DepositID, func(di DepositInfo) DepositInfo {
					di.Status = StatusDone
					di.Error = ErrEmptySendAmount.Error()
					return di
				})
				if err != nil {
					log.WithError(err).Error("Update DepositInfo set StatusDone failed")
					return di, err
				}

				log.WithError(ErrEmptySendAmount).Info("DepositInfo set to StatusDone")

				return di, nil
			}

			return di, err
		}

		// Find the coins from the skyTx
		// The skyTx contains one output sent to the destination address,
		// so this check is safe.
		// It is verified earlier by verifyCreatedTransaction
		var skySent uint64
		for _, o := range skyTx.Out {
			if o.Address.String() == di.SkyAddress {
				skySent = o.Coins
				break
			}
		}

		if skySent == 0 {
			err := errors.New("No output to destination address found in transaction")
			log.WithError(err).Error(err)
			return di, err
		}

		// Within a bolt.DB transaction, update the db then send the coins
		// If the send fails, the data is rolled back
		// If the db save fails, no coins had been sent
		di, err = s.store.UpdateDepositInfoCallback(di.DepositID, func(di DepositInfo) DepositInfo {
			di.Status = StatusWaitConfirm
			di.Txid = skyTx.TxIDHex()
			di.SkySent = skySent
			return di
		}, func(di DepositInfo) error {
			// NOTE: broadcastTransaction retries indefinitely on error
			// If the skycoin node is not reachable, this will block,
			// which will also block the database since it's in a transaction
			rsp, err := s.broadcastTransaction(skyTx)
			if err != nil {
				log.WithError(err).Error("broadcastTransaction failed")
				return err
			}

			// Invariant assertion: do not return this as an error, since
			// coins have been sent. This should never occur.
			if rsp.Txid != skyTx.TxIDHex() {
				log.Error("CRITICAL ERROR: BroadcastTxResponse.Txid != skyTx.TxIDHex()")
			}

			return nil
		})

		if err != nil {
			log.WithError(err).Error("store.UpdateDepositInfoCallback failed")
			return di, err
		}

		log.Info("DepositInfo set to StatusWaitConfirm")

		return di, nil

	case StatusWaitConfirm:
		// Wait for confirmation
		rsp := s.sender.IsTxConfirmed(di.Txid)

		if rsp == nil {
			log.WithError(ErrNoResponse).Warn("Sender closed")
			return di, ErrNoResponse
		}

		if rsp.Err != nil {
			log.WithError(rsp.Err).Error("IsTxConfirmed failed")
			return di, rsp.Err
		}

		if !rsp.Confirmed {
			log.Info("Transaction is not confirmed yet")
			return di, ErrNotConfirmed
		}

		log.Info("Transaction is confirmed")

		di, err := s.store.UpdateDepositInfo(di.DepositID, func(di DepositInfo) DepositInfo {
			di.Status = StatusDone
			return di
		})
		if err != nil {
			log.WithError(err).Error("UpdateDepositInfo set StatusDone failed")
			return di, err
		}

		log.Info("DepositInfo status set to StatusDone")

		return di, nil

	case StatusDone:
		log.Warn("DepositInfo already processed")
		return di, nil

	case StatusWaitDeposit:
		// We don't save any deposits with StatusWaitDeposit.
		// We can't transition to StatusWaitSend without a scanner.Deposit
		log.Error("StatusWaitDeposit cannot be processed and should never be handled by this method")
		fallthrough
	case StatusUnknown:
		fallthrough
	default:
		err := ErrDepositStatusInvalid
		log.WithError(err).Error(err)
		return di, err
	}
}

func (s *Send) calculateSkyDroplets(di DepositInfo) (uint64, error) {
	log := s.log
	var err error
	var skyAmt uint64
	switch di.CoinType {
	case scanner.CoinTypeBTC:
		skyAmt, err = CalculateBtcSkyValue(di.DepositValue, di.ConversionRate, s.cfg.MaxDecimals)
		if err != nil {
			log.WithError(err).Error("CalculateBtcSkyValue failed")
			return 0, err
		}
	case scanner.CoinTypeETH:
		//Gwei convert to wei, because stored-value is Gwei in case overflow of uint64
		skyAmt, err = CalculateEthSkyValue(mathutil.Gwei2Wei(di.DepositValue), di.ConversionRate, s.cfg.MaxDecimals)
		if err != nil {
			log.WithError(err).Error("CalculateEthSkyValue failed")
			return 0, err
		}
	default:
		log.WithError(scanner.ErrUnsupportedCoinType).Error()
		return 0, scanner.ErrUnsupportedCoinType
	}
	return skyAmt, nil
}

func (s *Send) createTransaction(di DepositInfo) (*coin.Transaction, error) {
	log := s.log.WithField("deposit", di)

	// This should never occur, the DepositInfo is saved with a SkyAddress
	// during GetOrCreateDepositInfo().
	if di.SkyAddress == "" {
		err := ErrNoBoundAddress
		log.WithError(err).Error(err)
		return nil, err
	}

	log = log.WithField("skyAddr", di.SkyAddress)
	log = log.WithField("skyRate", di.ConversionRate)
	log = log.WithField("maxDecimals", s.cfg.MaxDecimals)

	skyAmt, err := s.calculateSkyDroplets(di)
	if err != nil {
		log.WithError(err).Error("calculateSkyDroplets failed")
		return nil, err
	}
	skyAmtCoins, err := droplet.ToString(skyAmt)
	if err != nil {
		log.WithError(err).Error("droplet.ToString failed")
		return nil, err
	}

	log = log.WithField("sendAmtDroplets", skyAmt)
	log = log.WithField("sendAmtCoins", skyAmtCoins)

	log.Info("Creating skycoin transaction")

	if skyAmt == 0 {
		err := ErrEmptySendAmount
		log.WithError(err).Error(err)
		return nil, err
	}

	tx, err := s.sender.CreateTransaction(di.SkyAddress, skyAmt)
	if err != nil {
		log.WithError(err).Error("sender.CreateTransaction failed")
		return nil, err
	}

	log = log.WithField("transactionOutput", tx.Out)

	if err := verifyCreatedTransaction(tx, di, skyAmt); err != nil {
		log.WithError(err).Error("verifyCreatedTransaction failed")
		return nil, err
	}

	return tx, nil
}

func verifyCreatedTransaction(tx *coin.Transaction, di DepositInfo, skyAmt uint64) error {
	// Check invariant assertions:
	// The transaction should contain one output to the destination address.
	// It may or may not have a change output.
	count := 0

	for _, o := range tx.Out {
		if o.Address.String() != di.SkyAddress {
			continue
		}

		count++

		if o.Coins != skyAmt {
			return errors.New("CreateTransaction transaction coins are different")
		}
	}

	if count == 0 {
		return fmt.Errorf("CreateTransaction transaction has no output to address %s", di.SkyAddress)
	} else if count > 1 {
		return fmt.Errorf("CreateTransaction transaction has multiple outputs to address %s", di.SkyAddress)
	}

	return nil
}

func (s *Send) broadcastTransaction(tx *coin.Transaction) (*sender.BroadcastTxResponse, error) {
	log := s.log.WithField("txid", tx.TxIDHex())

	log.Info("Broadcasting skycoin transaction")

	rsp := s.sender.BroadcastTransaction(tx)

	log = log.WithField("sendRsp", rsp)

	if rsp == nil {
		err := ErrNoResponse
		log.WithError(err).Warn("Sender closed")
		return nil, err
	}

	if rsp.Err != nil {
		err := fmt.Errorf("Send skycoin failed: %v", rsp.Err)
		log.WithError(err).Error(err)
		return nil, err
	}

	log.Info("Sent skycoin")

	return rsp, nil
}

// Balance returns the number of coins left in the OTC wallet
func (s *Send) Balance() (*cli.Balance, error) {
	return s.sender.Balance()
}

func (s *Send) setStatus(err error) {
	defer s.statusLock.Unlock()
	s.statusLock.Lock()
	s.status = err
}

// Status returns the last return value of the processing state
func (s *Send) Status() error {
	defer s.statusLock.RUnlock()
	s.statusLock.RLock()
	return s.status
}
