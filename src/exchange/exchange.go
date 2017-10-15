// Package exchange manages the binded deposit address and skycoin address,
// when get new deposits from scanner, exchange will find the corresponding
// skycoin address, and use skycoin sender to send skycoins in given rate.
package exchange

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/boltdb/bolt"
	"github.com/sirupsen/logrus"

	"github.com/skycoin/teller/src/scanner"
	"github.com/skycoin/teller/src/sender"
	"github.com/skycoin/teller/src/util/dbutil"
)

const (
	satoshiPerBTC           int64 = 1e8
	txConfirmationCheckWait       = time.Second * 3
)

var (
	// ErrEmptySendAmount is returned if the calculated skycoin amount to send is 0
	ErrEmptySendAmount = errors.New("Skycoin send amount is 0")
	// ErrNoResponse is returned when the send service returns a nil response. This happens if the send service has closed.
	ErrNoResponse = errors.New("No response from the send service")
	// ErrNotConfirmed is returned if the tx is not confirmed yet
	ErrNotConfirmed = errors.New("Transaction is not confirmed yet")
)

// DepositFilter filters deposits
type DepositFilter func(di DepositInfo) bool

// Exchanger provides APIs to interact with the exchange service
type Exchanger interface {
	BindAddress(btcAddr, skyAddr string) error
	GetDepositStatuses(skyAddr string) ([]DepositStatus, error)
	GetDepositStatusDetail(flt DepositFilter) ([]DepositStatusDetail, error)
	GetBindNum(skyAddr string) (int, error)
}

// Exchange manages coin exchange between deposits and skycoin
type Exchange struct {
	log         logrus.FieldLogger
	cfg         Config
	scanner     scanner.Scanner // scanner provides APIs for interacting with the scan service
	sender      sender.Sender   // sender provides APIs for sending skycoin
	store       *store          // deposit info storage
	quit        chan struct{}
	done        chan struct{}
	depositChan chan DepositInfo
}

// Config exchange config struct
type Config struct {
	Rate                    int64 // sky_btc rate
	TxConfirmationCheckWait time.Duration
}

// NewExchange creates exchange service
func NewExchange(log logrus.FieldLogger, db *bolt.DB, scanner scanner.Scanner, sender sender.Sender, cfg Config) (*Exchange, error) {
	s, err := newStore(db, log)
	if err != nil {
		return nil, err
	}

	if cfg.Rate == 0 {
		return nil, errors.New("SKY/BTC Rate must not be 0")
	}

	if cfg.TxConfirmationCheckWait == 0 {
		cfg.TxConfirmationCheckWait = txConfirmationCheckWait
	}

	return &Exchange{
		cfg:         cfg,
		log:         log.WithField("prefix", "teller.exchange"),
		scanner:     scanner,
		sender:      sender,
		store:       s,
		quit:        make(chan struct{}),
		done:        make(chan struct{}, 1),
		depositChan: make(chan DepositInfo, 100),
	}, nil
}

// Run starts the exchange process
func (s *Exchange) Run() error {
	log := s.log
	log.Info("Start exchange service...")
	defer func() {
		log.Info("Closed exchange service")
		s.done <- struct{}{}
	}()

	// Load StatusWaitSend deposits for processing later
	waitSendDeposits, err := s.store.GetDepositInfoArray(func(di DepositInfo) bool {
		return di.Status == StatusWaitSend
	})

	if err != nil {
		err = fmt.Errorf("GetDepositInfoArray failed: %v", err)
		log.WithError(err).Error()
		return err
	}

	// Load StatusWaitConfirm deposits for processing later
	waitConfirmDeposits, err := s.store.GetDepositInfoArray(func(di DepositInfo) bool {
		return di.Status == StatusWaitConfirm
	})

	if err != nil {
		err = fmt.Errorf("GetDepositInfoArray failed: %v", err)
		log.WithError(err).Error()
		return err
	}

	var wg sync.WaitGroup

	// This loop processes StatusWaitSend deposits.
	// Only one deposit is processed at a time; it will not send more coins
	// until it receives confirmation of the previous send.
	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-s.quit:
				log.Info("exchange.Exchange send loop quit")
				return
			case d := <-s.depositChan:
				log := log.WithField("depositInfo", d)
				if err := s.processWaitSendDeposit(d); err != nil {
					log.WithError(err).Error("processWaitSendDeposit failed. This deposit will not be reprocessed until teller is restarted.")
				}
			}
		}
	}()

	// Queue the saved StatusWaitConfirm deposits
	for _, di := range waitConfirmDeposits {
		s.depositChan <- di
	}

	// Queue the saved StatusWaitSend deposits
	for _, di := range waitSendDeposits {
		s.depositChan <- di
	}

	// This loop processes incoming deposits from the scanner and saves a
	// new DepositInfo with a status of StatusWaitSend
	wg.Add(1)
	go func() {
		defer wg.Done()

		for {
			select {
			case <-s.quit:
				log.Info("exchange.Exchange watch deposits loop quit")
				return
			case dv, ok := <-s.scanner.GetDeposit():
				if !ok {
					log.Warn("Scan service closed, watch deposits loop quit")
					return
				}

				log := log.WithField("deposit", dv.Deposit)

				// Save a new DepositInfo based upon the scanner.Deposit.
				// If the save fails, report it to the scanner.
				// The scanner will mark the deposit as "processed" if no error
				// occurred.  Any unprocessed deposits held by the scanner
				// will be resent to the exchange when teller is started.
				if d, err := s.saveIncomingDeposit(dv.Deposit); err != nil {
					log.WithError(err).Error("saveIncomingDeposit failed. This deposit will not be reprocessed until teller is restarted")
					dv.ErrC <- err
				} else {
					dv.ErrC <- nil
					s.depositChan <- d
				}
			}
		}
	}()

	wg.Wait()

	return nil
}

// Shutdown close the exchange service
func (s *Exchange) Shutdown() {
	close(s.quit)
	s.log.Info("Waiting for Run() to finish")
	<-s.done
	s.log.Info("Shutdown complete")
}

// saveIncomingDeposit is called when receiving a deposit from the scanner
func (s *Exchange) saveIncomingDeposit(dv scanner.Deposit) (DepositInfo, error) {
	log := s.log.WithField("deposit", dv)

	log.Info("Received bitcoin deposit")

	di, err := s.getOrCreateDepositInfo(dv)
	if err != nil {
		log.WithError(err).Error("getOrCreateDepositInfo failed")
		return DepositInfo{}, err
	}

	log = log.WithField("depositInfo", di)
	log.Info("Saved DepositInfo")

	return di, err
}

// processDeposit advances a single deposit through three states:
// StatusWaitDeposit -> StatusWaitSend
// StatusWaitSend -> StatusWaitConfirm
// StatusWaitConfirm -> StatusDone
func (s *Exchange) processWaitSendDeposit(di DepositInfo) error {
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

		if di.Status == StatusDone {
			return nil
		}
	}

	return nil
}

func (s *Exchange) handleDepositInfoState(di DepositInfo) (DepositInfo, error) {
	log := s.log.WithField("depositInfo", di)

	switch di.Status {
	case StatusWaitDeposit:
		// Switch status to StatusWaitSend
		di, err := s.store.UpdateDepositInfo(di.BtcTx, func(di DepositInfo) DepositInfo {
			di.Status = StatusWaitSend
			return di
		})
		if err != nil {
			log.WithError(err).Error("UpdateDeposit set StatusWaitSend failed")
			return di, err
		}

		return di, nil

	case StatusWaitSend:
		// Send
		rsp, err := s.send(di)
		if err != nil {
			log.WithError(err).Error("Send failed")

			// If the send amount is empty, skip to StatusDone.
			switch err {
			case ErrNoResponse:
				log.WithError(err).Warn("Sender closed")
			case ErrEmptySendAmount:
				di, err = s.store.UpdateDepositInfo(di.BtcTx, func(di DepositInfo) DepositInfo {
					di.Status = StatusDone
					return di
				})
				if err != nil {
					log.WithError(err).Error("Update DepositInfo set StatusDone failed")
					return di, err
				}

				return di, nil
			default:
			}

			return di, err
		}

		// Update the txid
		di, err := s.store.UpdateDepositInfo(di.BtcTx, func(di DepositInfo) DepositInfo {
			di.Txid = rsp.Txid
			di.SkySent = rsp.Req.Coins
			di.Status = StatusWaitConfirm
			return di
		})
		if err != nil {
			log.WithError(err).Error("Update deposit info set StatusWaitConfirm failed")
			return di, err
		}

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

		di, err := s.store.UpdateDepositInfo(di.BtcTx, func(di DepositInfo) DepositInfo {
			di.Status = StatusDone
			return di
		})
		if err != nil {
			log.WithError(err).Error("UpdateDepositInfo set StatusDone failed")
			return di, err
		}

		return di, nil

	case StatusDone:
		log.Warn("DepositInfo already processed")
		return di, nil

	case StatusUnknown:
		fallthrough
	default:
		err := errors.New("Unknown deposit status")
		log.WithError(err).Error()
		return di, err
	}
}

func (s *Exchange) send(di DepositInfo) (*sender.SendResponse, error) {
	log := s.log.WithField("deposit", di)

	skyAddr, err := s.store.GetBindAddress(di.Deposit.Address)
	if err != nil {
		log.WithError(err).Error("GetBindAddress failed")
		return nil, err
	}

	if skyAddr == "" {
		err := errors.New("Deposit has no bound skycoin address")
		log.WithError(err).Error()
		return nil, err
	}

	log = log.WithField("skyAddr", skyAddr)
	log = log.WithField("skyRate", s.cfg.Rate)

	skyAmt, err := calculateSkyValue(di.Deposit.Value, s.cfg.Rate)
	if err != nil {
		log.WithError(err).Error("calculateSkyValue failed")
		return nil, err
	}

	log = log.WithField("sendAmt", skyAmt)

	log.Info("Trying to send skycoin")

	if skyAmt == 0 {
		log.WithError(err).Error()
		return nil, ErrEmptySendAmount
	}

	// TODO FIXME:
	// If UpdateDepositInfo fails we will send double coins.
	// We can't just wrap it all in a bolt.Tx, because we need to send the
	// skycoins first in order to obtain the Txid.
	// Can we generate the Txid offline, and specify it in the SendRequest?
	// We'd need the wallet loaded with teller.
	// Possibly we could make two API calls to skycoind, one to prepare the tx,
	// the other to send it.

	rsp := s.sender.Send(skyAddr, skyAmt)

	log = log.WithField("sendRsp", rsp)

	if rsp == nil {
		log.WithError(ErrNoResponse).Warn("Sender closed")
		return nil, ErrNoResponse
	}

	if rsp.Err != nil {
		err := fmt.Errorf("Send skycoin failed: %v", rsp.Err)
		log.WithError(err).Error()
		return nil, err
	}

	log.Info("Sent skycoin")

	return rsp, nil
}

func (s *Exchange) getOrCreateDepositInfo(dv scanner.Deposit) (DepositInfo, error) {
	log := s.log.WithField("deposit", dv)

	di, err := s.store.GetDepositInfo(dv.TxN())

	switch err.(type) {
	case nil:
		return di, nil

	case dbutil.ObjectNotExistErr:
		log.Info("DepositInfo not found in DB, inserting")
		di, err := s.createDepositInfo(dv)
		if err != nil {
			err = fmt.Errorf("createDepositInfo failed: %v", err)
			log.WithError(err).Error()
		}
		return di, err

	default:
		err = fmt.Errorf("GetDepositInfo failed: %v", err)
		log.WithError(err).Error()
		return DepositInfo{}, err
	}
}

func (s *Exchange) createDepositInfo(dv scanner.Deposit) (DepositInfo, error) {
	log := s.log.WithField("deposit", dv)

	skyAddr, err := s.store.GetBindAddress(dv.Address)
	if err != nil {
		err = fmt.Errorf("GetBindAddress failed: %v", err)
		log.WithError(err).Error()
		return DepositInfo{}, err
	}

	if skyAddr == "" {
		err := errors.New("Deposit has no bound skycoin address")
		log.WithError(err).Error()
		return DepositInfo{}, err
	}

	log = log.WithField("skyAddr", skyAddr)

	di := DepositInfo{
		SkyAddress: skyAddr,
		BtcAddress: dv.Address,
		BtcTx:      dv.TxN(),
		Status:     StatusWaitSend,
		Deposit:    dv,
		// Save the rate at the time this deposit was noticed
		SkyBtcRate: s.cfg.Rate,
	}

	log = log.WithField("depositInfo", di)

	if err := s.store.AddDepositInfo(di); err != nil {
		err = fmt.Errorf("AddDepositInfo failed: %v", err)
		log.WithError(err).Error()
		return DepositInfo{}, err
	}

	return di, nil
}

// BindAddress binds deposit btc address with skycoin address, and
// add the btc address to scan service, when detect deposit coin
// to the btc address, will send specific skycoin to the binded
// skycoin address
func (s *Exchange) BindAddress(btcAddr, skyAddr string) error {
	if err := s.store.BindAddress(skyAddr, btcAddr); err != nil {
		return err
	}

	// add btc address to scanner
	return s.scanner.AddScanAddress(btcAddr)
}

// DepositStatus json struct for deposit status
type DepositStatus struct {
	Seq       uint64 `json:"seq"`
	UpdatedAt int64  `json:"update_at"`
	Status    string `json:"status"`
}

// DepositStatusDetail deposit status detail info
type DepositStatusDetail struct {
	Seq        uint64 `json:"seq"`
	UpdatedAt  int64  `json:"update_at"`
	Status     string `json:"status"`
	SkyAddress string `json:"skycoin_address"`
	BtcAddress string `json:"bitcoin_address"`
	Txid       string `json:"txid"`
}

// GetDepositStatuses returns deamon.DepositStatus array of given skycoin address
func (s *Exchange) GetDepositStatuses(skyAddr string) ([]DepositStatus, error) {
	dis, err := s.store.GetDepositInfoOfSkyAddress(skyAddr)
	if err != nil {
		return []DepositStatus{}, err
	}

	dss := make([]DepositStatus, 0, len(dis))
	for _, di := range dis {
		dss = append(dss, DepositStatus{
			Seq:       di.Seq,
			UpdatedAt: di.UpdatedAt,
			Status:    di.Status.String(),
		})
	}
	return dss, nil
}

// GetDepositStatusDetail returns deposit status details
func (s *Exchange) GetDepositStatusDetail(flt DepositFilter) ([]DepositStatusDetail, error) {
	dis, err := s.store.GetDepositInfoArray(flt)
	if err != nil {
		return nil, err
	}

	dss := make([]DepositStatusDetail, 0, len(dis))
	for _, di := range dis {
		dss = append(dss, DepositStatusDetail{
			Seq:        di.Seq,
			UpdatedAt:  di.UpdatedAt,
			Status:     di.Status.String(),
			SkyAddress: di.SkyAddress,
			BtcAddress: di.BtcAddress,
			Txid:       di.Txid,
		})
	}
	return dss, nil
}

// GetBindNum returns the number of btc address the given sky address binded
func (s *Exchange) GetBindNum(skyAddr string) (int, error) {
	addrs, err := s.store.GetSkyBindBtcAddresses(skyAddr)
	return len(addrs), err
}
