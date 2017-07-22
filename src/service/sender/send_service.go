// Package sender provids send service for skycoin
package sender

import (
	"errors"
	"fmt"

	"time"

	"github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/teller/src/logger"
	"github.com/skycoin/teller/src/service/cli"
)

const sendCoinCheckTime = 3 * time.Second

// Request send coin request struct
type Request struct {
	Coins   int64            // coin number
	Address string           // recv address
	RspC    chan interface{} // response
}

// SendService is in charge of sending skycoin
type SendService struct {
	logger.Logger
	cfg     Config
	skycli  skyclient
	quit    chan struct{}
	reqChan chan Request
}

// Config sender configuration info
type Config struct {
	WalletPath   string // path to the ico coin wallet
	SkyRpcserver string // skycoin node server
	ReqBufSize   uint32 // the buffer size of sending request
}

type skyclient interface {
	Send(recvAddr string, coins int64) (string, error)
	GetTransaction(txid string) (*cli.TxJSON, error)
}

// NewService creates sender instance
func NewService(cfg Config, log logger.Logger, skycli skyclient) *SendService {
	return &SendService{
		Logger: log,
		cfg:    cfg,
		// skycli:  cli.New(cfg.walletPath, cfg.skyRpcserver),
		skycli:  skycli,
		quit:    make(chan struct{}),
		reqChan: make(chan Request, cfg.ReqBufSize),
	}
}

// Run start the send service
func (s *SendService) Run() error {
	go func() {
		defer s.Println("Send service exit")
		for {
			select {
			case req := <-s.reqChan:
				// verify the request
				if err := verifyRequest(req); err != nil {
					req.RspC <- sendRsp{err: fmt.Sprintf("Invalid request: %v", err)}
					continue
				}

			sendLoop:
				for { // loop to resend coin if send failed
					// sendCoin will not return until the tx is confirmed
					txid, err := s.sendCoin(req.Address, req.Coins)
					if err != nil {
						if err == ErrServiceClosed {
							// tx should be confirmed before shutdown
							if txid != "" {
								req.RspC <- sendRsp{txid: txid}
							}
							return
						}

						// transaction not create yet
						if txid == "" {
							s.Debugln("Send coin failed:", err, "try to send again..")
							time.Sleep(sendCoinCheckTime)
							continue
						}

						// transaction already exist, check tx status
						for {
							ok, err := s.isTxConfirmed(txid)
							if err != nil {
								s.Debugln(err)
								time.Sleep(sendCoinCheckTime)
								continue
							}

							if ok {
								req.RspC <- sendRsp{txid: txid}
								break sendLoop
							}
							time.Sleep(sendCoinCheckTime)
						}
					}
					req.RspC <- sendRsp{txid: txid}
					s.Printf("Send %d coins to %s success\n", req.Coins, req.Address)
					break
				}
			}
		}
	}()

	<-s.quit
	return nil
}

func verifyRequest(req Request) error {
	_, err := cipher.DecodeBase58Address(req.Address)
	if err != nil {
		return err
	}

	if req.Coins < 1 {
		return errors.New("Send coins must >= 1")
	}
	return nil
}

func (s *SendService) sendCoin(addr string, coins int64) (string, error) {
	// verify the address
	txid, err := s.skycli.Send(addr, coins)
	if err != nil {
		return "", err
	}

	errC := make(chan error, 1)
	okC := make(chan struct{}, 1)
	go func() {
		for {
			// check if the tx is confirmed
			tx, err := s.skycli.GetTransaction(txid)
			if err != nil {
				errC <- fmt.Errorf("Check confirmation of tx: %v failed: %v", txid, err)
				return
			}
			if tx.Transaction.Status.Confirmed {
				okC <- struct{}{}
				return
			}

			time.Sleep(sendCoinCheckTime)
		}
	}()

	select {
	case err := <-errC:
		return txid, err
	case <-okC:
		return txid, nil
	case <-s.quit:
		// wait until the transaction is confirmed, otherwise the
		<-okC
		return txid, ErrServiceClosed
	}
}

func (s *SendService) isTxConfirmed(txid string) (bool, error) {
	tx, err := s.skycli.GetTransaction(txid)
	if err != nil {
		return false, fmt.Errorf("Get transaction %s failed: %v", txid, err)
	}

	return tx.Transaction.Status.Confirmed, nil
}

// Shutdown close the sender
func (s *SendService) Shutdown() {
	close(s.quit)
}
