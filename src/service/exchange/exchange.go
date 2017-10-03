// Package exchange manages the binded deposit address and skycoin address,
// when get new deposits from scanner, exchange will find the corresponding
// skycoin address, and use skycoin sender to send skycoins in given rate.
package exchange

import (
	"errors"
	"time"

	"github.com/boltdb/bolt"
	"github.com/shopspring/decimal"

	skydaemon "github.com/skycoin/skycoin/src/daemon"
	"github.com/skycoin/skycoin/src/util/droplet"

	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/util/dbutil"
	"github.com/skycoin/teller/src/util/logger"
	"github.com/skycoin/teller/src/service/scanner"
	"github.com/skycoin/teller/src/service/sender"
)

const satoshiPerBTC int64 = 1e8

// SkySender provids apis for sending skycoin
type SkySender interface {
	SendAsync(destAddr string, coins uint64, opt *sender.SendOption) (<-chan interface{}, error)
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
	sky = sky.Truncate(skydaemon.MaxDropletPrecision)

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
	logger.Logger
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
func NewService(cfg Config, db *bolt.DB, log logger.Logger, scanner BtcScanner, sender SkySender) *Service {
	s, err := newStore(db)
	if err != nil {
		panic(err)
	}

	return &Service{
		cfg:     cfg,
		Logger:  log,
		scanner: scanner,
		sender:  sender,
		store:   s,
		quit:    make(chan struct{}),
	}
}

// Run starts the exchange process
func (s *Service) Run() error {
	s.Println("Start exchange service...")
	defer s.Println("Exchange service closed")

	s.processUnconfirmedTx()

	for {
		select {
		case <-s.quit:
			s.Println("exhange.Service quit")
			return nil
		case dv, ok := <-s.scanner.GetDepositValue():
			if !ok {
				s.Println("Scan service closed")
				return nil
			}

			s.Printf("Receive %f deposit bitcoin from %s\n", dv.Value, dv.Address)
			btcTxIndex := dv.TxN()
			// get deposit info of given btc address
			di, err := s.store.GetDepositInfo(btcTxIndex)

			switch err.(type) {
			case nil:
			case dbutil.ObjectNotExistErr:
				// s.Printf("Deposit info from btc address %s with tx %s doesn't exist\n", dv.Address, dv.Tx)
				// continue
				skyAddr, err := s.store.GetBindAddress(dv.Address)
				if err != nil {
					s.Printf("GetBindAddress failed: %+v %v\n", dv, err)
					continue
				}

				if skyAddr == "" {
					s.Printf("Deposit from %s has no bind skycoin address\n", dv.Address)
					dv.AckC <- struct{}{}
					continue
				}

				di = DepositInfo{
					SkyAddress: skyAddr,
					BtcAddress: dv.Address,
					BtcTx:      btcTxIndex,
					Status:     StatusWaitSend,
				}

				if err := s.store.AddDepositInfo(di); err != nil {
					s.Printf("Add deposit info %v failed: %v\n", di, err)
					continue
				}
			default:
				s.Printf("GetDepositInfo failed: %s %v", btcTxIndex, err)
				continue
			}

			if di.Status >= StatusWaitConfirm {
				dv.AckC <- struct{}{}
				s.Printf("Deposit info from btc address %s with tx %s already processed\n", dv.Address, dv.Tx)
				continue
			}

			if di.Status == StatusWaitDeposit {
				// update status to waiting_sky_send
				if err := s.store.UpdateDepositInfo(btcTxIndex, func(dpi DepositInfo) DepositInfo {
					dpi.Status = StatusWaitSend
					return dpi
				}); err != nil {
					s.Printf("Update deposit status of btc tx %s failed: %v\n", dv.Tx, err)
					continue
				}
			}

			// send skycoins
			// get binded skycoin address
			skyAddr, err := s.store.GetBindAddress(dv.Address)
			if err != nil {
				s.Printf("GetBindAddress failed: %+v %v\n", dv, err)
				continue
			}

			if skyAddr == "" {
				s.Println("No bound skycoin address found for btc address", dv.Address)
				continue
			}

			// checks if the send service is closed
			if s.sender.IsClosed() {
				s.Println("Send service closed")
				return nil
			}

			// try to send skycoin
			skyAmt, err := calculateSkyValue(dv.Value, s.cfg.Rate)
			if err != nil {
				s.Printf("calculateSkyValue error: %v", err)
				continue
			}

			s.Printf("Trying to send %d skycoin droplets to %s\n", skyAmt, skyAddr)

			if skyAmt == 0 {
				s.Printf("skycoin amount is 0, not sending")
				continue
			}

			rspC, _ := s.sender.SendAsync(skyAddr, skyAmt, nil)
			var rsp sender.Response
			select {
			case r := <-rspC:
				rsp = r.(sender.Response)
			case <-s.quit:
				s.Println("exhange.Service quit")
				return nil
			}

			if rsp.Err != "" {
				s.Println("Send skycoin failed:", rsp.Err)
				dv.AckC <- struct{}{}
				continue
			}

			s.Printf("Sent %d skycoin droplets to %s\n", skyAmt, skyAddr)

			// update the txid
			if err := s.store.UpdateDepositInfo(btcTxIndex, func(dpi DepositInfo) DepositInfo {
				dpi.Txid = rsp.Txid
				dpi.SkySent = skyAmt
				dpi.SkyBtcRate = s.cfg.Rate
				return dpi
			}); err != nil {
				s.Printf("Update deposit info failed: btcAddr=%s rate=%d skySent=%d txid=%s err=%v\n", dv.Address, s.cfg.Rate, skyAmt, rsp.Txid, err)
			}

		loop:
			for {
				select {
				case <-s.quit:
					s.Println("exhange.Service quit")
					return nil
				case st := <-rsp.StatusC:
					switch st {
					case sender.Sent:
						s.Printf("Status=%s, skycoin address=%s\n", StatusWaitConfirm, skyAddr)
						if err := s.store.UpdateDepositInfo(btcTxIndex, func(dpi DepositInfo) DepositInfo {
							dpi.Status = StatusWaitConfirm
							return dpi
						}); err != nil {
							s.Printf("Update deposit info for btc address %s failed: %v\n", dv.Address, err)
						}
					case sender.TxConfirmed:
						if err := s.store.UpdateDepositInfo(btcTxIndex, func(dpi DepositInfo) DepositInfo {
							dpi.Status = StatusDone
							return dpi
						}); err != nil {
							s.Printf("Update deposit info for btc address %s failed: %v\n", dv.Address, err)
						}

						dv.AckC <- struct{}{}

						s.Printf("Status=%s, txid=%s, skycoin address=%s\n", StatusDone, rsp.Txid, skyAddr)

						break loop
					default:
						s.Panicln("Unknown sender.Response.StatusC value", st)
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
	s.Println("Checking the unconfirmed tx...")
	defer s.Println("Checking confirmed tx finished")

	dpis, err := s.store.GetDepositInfoArray(func(dpi DepositInfo) bool {
		return dpi.Status == StatusWaitConfirm
	})

	if err != nil {
		s.Println("GetDepositInfoArray failed:", err)
		return
	}

	if len(dpis) == 0 {
		return
	}

	for _, dpi := range dpis {
		// check if the tx is confirmed
	loop:
		for {
			if s.sender.IsTxConfirmed(dpi.Txid) {
				// update the dpi status
				if err := s.store.UpdateDepositInfo(dpi.BtcTx, func(dpi DepositInfo) DepositInfo {
					dpi.Status = StatusDone
					return dpi
				}); err != nil {
					s.Println("Update deposit info status in ProcessUnconfirmedTx failed:", err)
				}
				break loop
			}

			s.Debugf("Txid %s is not confirmed...", dpi.Txid)
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

func (s *Service) getDepositStatuses(skyAddr string) ([]daemon.DepositStatus, error) {
	dpis, err := s.store.GetDepositInfoOfSkyAddress(skyAddr)
	if err != nil {
		return []daemon.DepositStatus{}, err
	}

	dss := make([]daemon.DepositStatus, 0, len(dpis))
	for _, dpi := range dpis {
		dss = append(dss, daemon.DepositStatus{
			Seq:      dpi.Seq,
			UpdateAt: dpi.UpdatedAt,
			Status:   dpi.Status.String(),
		})
	}
	return dss, nil
}

// DepositFilter deposit status filter
type DepositFilter func(dpi DepositInfo) bool

func (s *Service) getDepositStatusDetail(flt DepositFilter) ([]daemon.DepositStatusDetail, error) {
	dpis, err := s.store.GetDepositInfoArray(flt)
	if err != nil {
		return nil, err
	}

	dss := make([]daemon.DepositStatusDetail, 0, len(dpis))
	for _, dpi := range dpis {
		dss = append(dss, daemon.DepositStatusDetail{
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
