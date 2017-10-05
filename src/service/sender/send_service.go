// Package sender provids send service for skycoin
package sender

import (
	"errors"
	"fmt"
	"sync"

	"time"

	"github.com/sirupsen/logrus"
	"github.com/skycoin/skycoin/src/api/webrpc"
	"github.com/skycoin/skycoin/src/cipher"
)

const sendCoinCheckTime = 3 * time.Second

// SendStatus represents the send status
type SendStatus int8

const (
	// Sent represents coins already sent, but waiting confirm
	Sent SendStatus = iota + 1
	// TxConfirmed represents the transaction is confirmed
	TxConfirmed
)

// Request send coin request struct
type Request struct {
	Coins   uint64           // coin number (in droplets)
	Address string           // recv address
	RspC    chan interface{} // response
}

// Response send response
type Response struct {
	Err     string
	Txid    string
	StatusC chan SendStatus
}

func makeResponse(txid string, err string) Response {
	return Response{
		Txid:    txid,
		Err:     err,
		StatusC: make(chan SendStatus, 5),
	}
}

// SendService is in charge of sending skycoin
type SendService struct {
	log      logrus.FieldLogger
	cfg      Config
	skycli   skyclient
	quit     chan struct{}
	reqChan  chan Request
	isClosed bool
	sync.Mutex
}

// Config sender configuration info
type Config struct {
	ReqBufSize uint32 // the buffer size of sending request
}

type skyclient interface {
	Send(recvAddr string, coins uint64) (string, error)
	GetTransaction(txid string) (*webrpc.TxnResult, error)
}

// NewService creates sender instance
func NewService(cfg Config, log logrus.FieldLogger, skycli skyclient) *SendService {
	return &SendService{
		log: log.WithFields(logrus.Fields{
			"prefix": "sender",
			"obj":    "SendService",
		}),
		cfg:     cfg,
		skycli:  skycli,
		quit:    make(chan struct{}),
		reqChan: make(chan Request, cfg.ReqBufSize),
	}
}

// Run start the send service
func (s *SendService) Run() error {
	log := s.log.WithField("func", "Run")

	log.Info("Start skycoin send service")
	defer log.Info("Skycoin send service closed")

	for {
		select {
		case <-s.quit:
			return nil
		case req := <-s.reqChan:
			// verify the request
			if err := verifyRequest(req); err != nil {
				req.RspC <- Response{
					Err: fmt.Sprintf("Invalid request: %v", err),
				}
				continue
			}

		sendLoop:
			for { // loop to resend coin if send failed

				txid, err := s.skycli.Send(req.Address, req.Coins)
				if err != nil {
					log.WithError(err).Error("skycli.Send failed, trying again...")

					select {
					case <-s.quit:
						return nil
					case <-time.After(sendCoinCheckTime):
					}

					continue
				}

				rsp := makeResponse(txid, "")
				go func() {
					rsp.StatusC <- Sent
				}()

				req.RspC <- rsp

				// transaction already exist, check tx status
				for {
					ok, err := s.isTxConfirmed(txid)
					if err != nil {
						log.WithError(err).Error("isTxConfirmed failed, trying again...")

						select {
						case <-s.quit:
							return nil
						case <-time.After(sendCoinCheckTime):
						}

						continue
					}

					if ok {
						go func() {
							rsp.StatusC <- TxConfirmed
						}()
						break sendLoop
					}

					select {
					case <-s.quit:
						return nil
					case <-time.After(sendCoinCheckTime):
					}
				}
			}
		}
	}
}

func verifyRequest(req Request) error {
	_, err := cipher.DecodeBase58Address(req.Address)
	if err != nil {
		return err
	}

	if req.Coins < 1 {
		return errors.New("send coins must >= 1")
	}

	return nil
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
	s.Lock()
	s.isClosed = true
	s.Unlock()
	close(s.quit)
}

// IsClosed checks if the send service is closed
func (s *SendService) IsClosed() bool {
	s.Lock()
	defer s.Unlock()
	return s.isClosed
}
