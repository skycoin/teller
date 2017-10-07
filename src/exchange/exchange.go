// Package exchange manages the binded deposit address and skycoin address,
// when get new deposits from scanner, exchange will find the corresponding
// skycoin address, and use skycoin sender to send skycoins in given rate.
package exchange

import (
	"errors"
	"time"

	"github.com/boltdb/bolt"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"

	"github.com/skycoin/skycoin/src/daemon"
	"github.com/skycoin/skycoin/src/util/droplet"

	"github.com/skycoin/teller/src/scanner"
	"github.com/skycoin/teller/src/sender"
	"github.com/skycoin/teller/src/util/dbutil"
)

const satoshiPerBTC int64 = 1e8

// SkySender provids apis for sending skycoin
type SkySender interface {
	SendAsync(destAddr string, coins uint64) <-chan sender.Response
	IsTxConfirmed(txid string) bool
	IsClosed() bool
}

// BtcScanner provids apis for interact with scan service
type BtcScanner interface {
	AddScanAddress(addr string) error
	GetScanAddresses() ([]string, error)
	GetDepositValue() <-chan scanner.DepositNote
}

// calculateSkyValue returns the amount of SKY (in droplets) to give for an
// amount of BTC (in satoshis).
// Rate is measured in SKY per BTC.
func calculateSkyValue(satoshis, skyPerBTC int64) (uint64, error) {
	if satoshis < 0 || skyPerBTC < 0 {
		return 0, errors.New("negative satoshis or negative skyPerBTC")
	}

	btc := decimal.New(satoshis, 0)
	btcToSatoshi := decimal.New(satoshiPerBTC, 0)
	btc = btc.DivRound(btcToSatoshi, 8)

	rate := decimal.New(skyPerBTC, 0)

	sky := btc.Mul(rate)
	sky = sky.Truncate(daemon.MaxDropletPrecision)

	skyToDroplets := decimal.New(droplet.Multiplier, 0)
	droplets := sky.Mul(skyToDroplets)

	amt := droplets.IntPart()
	if amt < 0 {
		// This should never occur, but double check before we convert to uint64,
		// otherwise we would send all the coins due to integer wrapping.
		return 0, errors.New("calculated sky amount is negative")
	}

	return uint64(amt), nil
}

// Service manages coin exchange between deposits and skycoin
type Service struct {
	log     logrus.FieldLogger
	cfg     Config
	scanner BtcScanner // scanner provides apis for interacting with scan service
	sender  SkySender  // sender provides apis for sending skycoin
	store   *store     // deposit info storage
	quit    chan struct{}
}

// Config exchange config struct
type Config struct {
	Rate int64 // sky_btc rate
}

// NewService creates exchange service
func NewService(cfg Config, db *bolt.DB, log logrus.FieldLogger, scanner BtcScanner, sender SkySender) *Service {
	s, err := newStore(db, log)
	if err != nil {
		panic(err)
	}

	return &Service{
		cfg:     cfg,
		log:     log.WithField("prefix", "teller.exchange"),
		scanner: scanner,
		sender:  sender,
		store:   s,
		quit:    make(chan struct{}),
	}
}

// Run starts the exchange process
func (s *Service) Run() error {
	log := s.log
	log.Info("Start exchange service...")
	defer log.Info("Exchange service closed")

	s.processUnconfirmedTx()

	for {
		select {
		case <-s.quit:
			log.Info("exhange.Service quit")
			return nil
		case dv, ok := <-s.scanner.GetDepositValue():
			log = log.WithField("depositValue", dv)

			if !ok {
				log.Warn("Scan service closed")
				return nil
			}

			log.Info("Receive bitcoin deposit")
			btcTxIndex := dv.TxN()
			// get deposit info of given btc address
			di, err := s.store.GetDepositInfo(btcTxIndex)

			// TODO, can rewrite this method a little better
			switch err.(type) {
			case nil:
				log = log.WithField("depositInfo", di)
			case dbutil.ObjectNotExistErr:
				log.Info("DepositInfo not found in DB, attempting to backfill")

				skyAddr, err := s.store.GetBindAddress(dv.Address)
				if err != nil {
					log.WithError(err).Error("GetBindAddress failed")
					continue
				}

				if skyAddr == "" {
					log.Warn("Deposit has no bound skycoin address")
					dv.AckC <- struct{}{}
					continue
				}

				log = log.WithField("skyAddr", skyAddr)

				di = DepositInfo{
					SkyAddress: skyAddr,
					BtcAddress: dv.Address,
					BtcTx:      btcTxIndex,
					Status:     StatusWaitSend,
				}

				log = log.WithField("depositInfo", di)

				if err := s.store.AddDepositInfo(di); err != nil {
					log.WithError(err).Error("Add DepositInfo failed")
					continue
				}
			default:
				log.WithError(err).Error("GetDepositInfo failed")
				continue
			}

			if di.Status >= StatusWaitConfirm {
				dv.AckC <- struct{}{}
				log.Warn("DepositInfo already processed")
				continue
			}

			if di.Status == StatusWaitDeposit {
				// update status to waiting_sky_send
				if err := s.store.UpdateDepositInfo(btcTxIndex, func(dpi DepositInfo) DepositInfo {
					dpi.Status = StatusWaitSend
					return dpi
				}); err != nil {
					log.WithError(err).Error("Update DepositStatus failed")
					continue
				}
			}

			// send skycoins
			// get bound skycoin address
			skyAddr, err := s.store.GetBindAddress(dv.Address)
			if err != nil {
				log.WithError(err).Error("GetBindAddress failed")
				continue
			}

			if skyAddr == "" {
				log.Error("Deposit has no bound skycoin address")
				continue
			}

			log = log.WithField("skyAddr", skyAddr)

			// checks if the send service is closed
			if s.sender.IsClosed() {
				log.Warn("Send service closed")
				return nil
			}

			log = log.WithField("skyRate", s.cfg.Rate)

			// try to send skycoin
			skyAmt, err := calculateSkyValue(dv.Value, s.cfg.Rate)
			if err != nil {
				log.WithError(err).Error("calculateSkyValue failed")
				continue
			}

			log = log.WithField("sendSkyDroplets", skyAmt)

			log.Info("Trying to send skycoin")

			if skyAmt == 0 {
				log.Warn("skycoin amount is 0, not sending")
				continue
			}

			rspC := s.sender.SendAsync(skyAddr, skyAmt)
			var rsp sender.Response
			select {
			case rsp = <-rspC:
			case <-s.quit:
				log.Warn("exhange.Service quit")
				return nil
			}

			log = log.WithField("response", rsp)

			if rsp.Err != "" {
				log.Error("Send skycoin failed")
				dv.AckC <- struct{}{}
				continue
			}

			log.Info("Sent skycoin")

			// update the txid
			if err := s.store.UpdateDepositInfo(btcTxIndex, func(dpi DepositInfo) DepositInfo {
				dpi.Txid = rsp.Txid
				dpi.SkySent = skyAmt
				dpi.SkyBtcRate = s.cfg.Rate
				return dpi
			}); err != nil {
				log.WithError(err).Error("Update deposit info failed")
			}

		loop:
			for {
				select {
				case <-s.quit:
					log.Warn("exhange.Service quit")
					return nil
				case st := <-rsp.StatusC:
					log = log.WithField("responseStatus", st)
					switch st {
					case sender.Sent:
						log = log.WithField("exchangeStatus", StatusWaitConfirm)
						log.Info("Handling response status channel result")

						if err := s.store.UpdateDepositInfo(btcTxIndex, func(dpi DepositInfo) DepositInfo {
							dpi.Status = StatusWaitConfirm
							return dpi
						}); err != nil {
							log.WithError(err).Error("Update DepositInfo failed")
						}
					case sender.TxConfirmed:
						log = log.WithField("exchangeStatus", StatusDone)
						log.Info("Handling response status channel result")

						if err := s.store.UpdateDepositInfo(btcTxIndex, func(dpi DepositInfo) DepositInfo {
							dpi.Status = StatusDone
							return dpi
						}); err != nil {
							log.WithError(err).Error("Update DepositInfo failed")
						}

						dv.AckC <- struct{}{}

						break loop
					default:
						log.Panic("Unknown responseStatus value")
						return nil
					}
				}

			}

		}
	}
}

// Shutdown close the exchange service
func (s *Service) Shutdown() {
	close(s.quit)
}

// ProcessUnconfirmedTx wait until all unconfirmed tx to be confirmed and update
// it's status in db
func (s *Service) processUnconfirmedTx() {
	log := s.log
	log.Info("Checking the unconfirmed tx...")
	defer log.Info("Checking confirmed tx finished")

	dpis, err := s.store.GetDepositInfoArray(func(dpi DepositInfo) bool {
		return dpi.Status == StatusWaitConfirm
	})

	if err != nil {
		log.WithError(err).Println("GetDepositInfoArray failed")
		return
	}

	log = log.WithField("depositInfosLen", len(dpis))

	if len(dpis) == 0 {
		return
	}

	for _, dpi := range dpis {
		dpiLog := log.WithField("depositInfo", dpi)
		// check if the tx is confirmed
	loop:
		for {
			if s.sender.IsTxConfirmed(dpi.Txid) {
				// update the dpi status
				if err := s.store.UpdateDepositInfo(dpi.BtcTx, func(dpi DepositInfo) DepositInfo {
					dpi.Status = StatusDone
					return dpi
				}); err != nil {
					dpiLog.WithError(err).Error("Update DepositInfo.Status")
				}
				break loop
			}

			dpiLog.Debug("Txid is not confirmed")
			select {
			case <-time.After(3 * time.Second):
				continue
			case <-s.quit:
				return
			}
		}
	}
}

func (s *Service) bindAddress(btcAddr, skyAddr string) error {
	if err := s.store.BindAddress(skyAddr, btcAddr); err != nil {
		return err
	}

	// add btc address to scanner
	return s.scanner.AddScanAddress(btcAddr)
}

// DepositStatus json struct for deposit status
type DepositStatus struct {
	Seq      uint64 `json:"seq"`
	UpdateAt int64  `json:"update_at"`
	Status   string `json:"status"`
}

// DepositStatusDetail deposit status detail info
type DepositStatusDetail struct {
	Seq        uint64 `json:"seq"`
	UpdateAt   int64  `json:"update_at"`
	Status     string `json:"status"`
	SkyAddress string `json:"skycoin_address"`
	BtcAddress string `json:"bitcoin_address"`
	Txid       string `json:"txid"`
}

func (s *Service) getDepositStatuses(skyAddr string) ([]DepositStatus, error) {
	dpis, err := s.store.GetDepositInfoOfSkyAddress(skyAddr)
	if err != nil {
		return []DepositStatus{}, err
	}

	dss := make([]DepositStatus, 0, len(dpis))
	for _, dpi := range dpis {
		dss = append(dss, DepositStatus{
			Seq:      dpi.Seq,
			UpdateAt: dpi.UpdatedAt,
			Status:   dpi.Status.String(),
		})
	}
	return dss, nil
}

// DepositFilter deposit status filter
type DepositFilter func(dpi DepositInfo) bool

func (s *Service) getDepositStatusDetail(flt DepositFilter) ([]DepositStatusDetail, error) {
	dpis, err := s.store.GetDepositInfoArray(flt)
	if err != nil {
		return nil, err
	}

	dss := make([]DepositStatusDetail, 0, len(dpis))
	for _, dpi := range dpis {
		dss = append(dss, DepositStatusDetail{
			Seq:        dpi.Seq,
			UpdateAt:   dpi.UpdatedAt,
			Status:     dpi.Status.String(),
			SkyAddress: dpi.SkyAddress,
			BtcAddress: dpi.BtcAddress,
			Txid:       dpi.Txid,
		})
	}
	return dss, nil
}

func (s *Service) getBindNum(skyAddr string) (int, error) {
	addrs, err := s.store.GetSkyBindBtcAddresses(skyAddr)
	return len(addrs), err
}
