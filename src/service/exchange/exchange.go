// Package exchange manages the binded deposit address and skycoin address,
// when get new deposits from scanner, exchange will find the corresponding
// skycoin address, and use skycoin sender to send skycoins in given rate.
package exchange

import (
	"github.com/boltdb/bolt"
	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/logger"
	"github.com/skycoin/teller/src/service/scanner"
	"github.com/skycoin/teller/src/service/sender"
)

// SkySender provids apis for sending skycoin
type SkySender interface {
	SendAsync(destAddr string, coins int64, opt *sender.SendOption) (<-chan interface{}, error)
	IsClosed() bool
}

// BtcScanner provids apis for interact with scan service
type BtcScanner interface {
	AddDepositAddress(addr string) error
	GetDepositAddresses() []string
	GetDepositValue() <-chan scanner.DepositValue
}

func calculateSkyValue(btcValue float64, rate float64) int64 {
	return int64(btcValue * rate)
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

			// get deposit info of given btc address
			_, ok = s.store.GetDepositInfo(dv.Address)
			if !ok {
				s.Printf("Deposit info of btc address %s doesn't exist\n", dv.Address)
				continue
			}

			// update status to waiting_sky_send
			err := s.store.UpdateDepositInfo(dv.Address, func(dpi DepositInfo) DepositInfo {
				dpi.Status = StatusWaitSend
				return dpi
			})

			if err != nil {
				s.Printf("Update deposit status of btc address %s failed: %v\n", dv.Address, err)
				continue
			}

			// send skycoins
			// get binded skycoin address
			skyAddr, ok := s.store.GetBindAddress(dv.Address)
			if !ok {
				s.Println("Find no bind skycoin address for btc address", dv.Address)
				continue
			}

			// checks if the send service is closed
			if s.sender.IsClosed() {
				s.Println("Send service closed")
				return nil
			}

			// try to send skycoin
			skyAmt := calculateSkyValue(dv.Value, float64(s.cfg.Rate))
			s.Printf("Send %d skycoin to %s\n", skyAmt, skyAddr)
			rspC, err := s.sender.SendAsync(skyAddr, skyAmt, nil)

			rsp := (<-rspC).(sender.Response)
			if rsp.Err != "" {
				s.Println("Send skycoin failed:", rsp.Err)
				continue
			}

		loop:
			for {
				st := <-rsp.StatusC
				switch st {
				case sender.Sent:
					s.Printf("Status=%s, skycoin address=%s\n", statusString[StatusWaitConfirm], skyAddr)
					if err := s.store.UpdateDepositInfo(dv.Address, func(dpi DepositInfo) DepositInfo {
						dpi.Status = StatusWaitConfirm
						return dpi
					}); err != nil {
						s.Printf("Update deposit info for btc address %s failed: %v\n", dv.Address, err)
					}
				case sender.TxConfirmed:
					s.Printf("Status=%s, skycoin address=%s\n", statusString[StatusDone], skyAddr)
					if err := s.store.UpdateDepositInfo(dv.Address, func(dpi DepositInfo) DepositInfo {
						dpi.Status = StatusDone
						return dpi
					}); err != nil {
						s.Printf("Update deposit info for btc address %s failed: %v\n", dv.Address, err)
					}
					break loop
				default:
					s.Panicln("Unknown sender.Response.StatusC value", st)
					return nil
				}
			}

			s.Printf("Send %d skycoin to %s success, txid=%s, deposit address=%s\n",
				skyAmt, skyAddr, rsp.Txid, dv.Address)

			// update the txid
			if err := s.store.UpdateDepositInfo(dv.Address, func(dpi DepositInfo) DepositInfo {
				dpi.Status = StatusDone
				dpi.Txid = rsp.Txid
				return dpi
			}); err != nil {
				s.Printf("Update deposit info for btc address %s failed: %v\n", dv.Address, err)
			}
		}
	}
}

// Shutdown close the exchange service
func (s *Service) Shutdown() {
	close(s.quit)
}

func (s *Service) addDepositInfo(btcAddr, skyAddr string) error {
	if _, err := s.store.AddDepositInfo(DepositInfo{
		BtcAddress: btcAddr,
		SkyAddress: skyAddr,
	}); err != nil {
		return err
	}

	// add btc address to scanner
	return s.scanner.AddDepositAddress(btcAddr)
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
	dpis := s.store.GetDepositInfoArray(flt)

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

func (s *Service) getBindNum(skyAddr string) int {
	return len(s.store.GetSkyBindBtcAddresses(skyAddr))
}
