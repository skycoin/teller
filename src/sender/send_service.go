// Package sender provids send service for skycoin
package sender

import (
	"errors"

	"time"

	"github.com/sirupsen/logrus"
	"github.com/skycoin/skycoin/src/api/webrpc"
	"github.com/skycoin/skycoin/src/cipher"
)

const (
	sendCoinRetryWait  = 3 * time.Second
	confirmTxRetryWait = 3 * time.Second
)

// SendRequest send coin request struct
type SendRequest struct {
	Coins   uint64             // coin number (in droplets)
	Address string             // recv address
	RspC    chan *SendResponse // response
}

// Verify verifies the request parameters
func (r SendRequest) Verify() error {
	_, err := cipher.DecodeBase58Address(r.Address)
	if err != nil {
		return err
	}

	if r.Coins < 1 {
		return errors.New("send coins must >= 1")
	}

	return nil
}

// SendResponse send response
type SendResponse struct {
	Txid string
	Err  error
	Req  SendRequest
}

// ConfirmRequest tx confirmation request struct
type ConfirmRequest struct {
	Txid string
	RspC chan *ConfirmResponse
}

// Verify verifies the request parameters
func (r ConfirmRequest) Verify() error {
	if r.Txid == "" {
		return errors.New("Txid empty")
	}

	return nil
}

// ConfirmResponse tx confirmation response
type ConfirmResponse struct {
	Confirmed bool
	Err       error
	Req       ConfirmRequest
}

// SendService is in charge of sending skycoin
type SendService struct {
	log         logrus.FieldLogger
	cfg         Config
	skycli      skyclient
	quit        chan struct{}
	sendChan    chan SendRequest
	confirmChan chan ConfirmRequest
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
		log:      log.WithField("prefix", "sender.service"),
		cfg:      cfg,
		skycli:   skycli,
		quit:     make(chan struct{}),
		sendChan: make(chan SendRequest, cfg.ReqBufSize),
	}
}

// Run start the send service
func (s *SendService) Run() error {
	log := s.log
	log.Info("Start skycoin send service")
	defer log.Info("Skycoin send service closed")

	for {
		select {
		case <-s.quit:
			return nil
		case req := <-s.sendChan:
			rsp, err := s.SendRetry(req)
			if err != nil {
				log.WithError(err).Error("SendRetry failed")
				req.RspC <- &SendResponse{
					Req: req,
					Err: err,
				}
			} else {
				req.RspC <- rsp
			}
		case req := <-s.confirmChan:
			rsp, err := s.ConfirmRetry(req)
			if err != nil {
				log.WithError(err).Error("ConfirmRetry failed")
				req.RspC <- &ConfirmResponse{
					Req: req,
					Err: err,
				}
			} else {
				req.RspC <- rsp
			}
		}
	}
}

// Confirm confirms a transaction
func (s *SendService) Confirm(req ConfirmRequest) (*ConfirmResponse, error) {
	log := s.log.WithField("confirmReq", req)

	if err := req.Verify(); err != nil {
		log.WithError(err).Error("ConfirmRequest.Verify failed")
		return nil, err
	}

	tx, err := s.skycli.GetTransaction(req.Txid)
	if err != nil {
		log.WithError(err).Error("skycli.GetTransaction failed")
		return nil, err
	}

	return &ConfirmResponse{
		Confirmed: tx.Transaction.Status.Confirmed,
		Req:       req,
	}, nil
}

// ConfirmRetry confirms a transaction and will retry indefinitely until it succeeds
func (s *SendService) ConfirmRetry(req ConfirmRequest) (*ConfirmResponse, error) {
	log := s.log.WithField("confirmReq", req)

	if err := req.Verify(); err != nil {
		log.WithError(err).Error("ConfirmRequest.Verify failed")
		return nil, err
	}

	// This loop tries to confirm the transaction until it succeeds.
	// TODO: if this gets stuck, nothing will proceed.
	// Add logic to give up confirmation after some number of retries, if necessary.
	// Most likely reason for GetTransaction() to fail is because the skyd node
	// is unavailable.
	for {
		tx, err := s.skycli.GetTransaction(req.Txid)
		if err != nil {
			log.WithError(err).Error("skycli.GetTransaction failed, trying again...")

			select {
			case <-s.quit:
				return nil, nil
			case <-time.After(confirmTxRetryWait):
			}

			continue
		}

		return &ConfirmResponse{
			Confirmed: tx.Transaction.Status.Confirmed,
			Req:       req,
		}, nil
	}
}

// Send sends coins
func (s *SendService) Send(req SendRequest) (*SendResponse, error) {
	log := s.log.WithField("sendReq", req)

	// Verify the request
	if err := req.Verify(); err != nil {
		log.WithError(err).Error("SendRequest.Verify failed")
		return nil, err
	}

	txid, err := s.skycli.Send(req.Address, req.Coins)
	if err != nil {
		log.WithError(err).Error("skycli.Send failed")
		return nil, err
	}

	return &SendResponse{
		Txid: txid,
		Req:  req,
	}, nil
}

// SendRetry sends coins and will retry indefinitely until it succeeds
func (s *SendService) SendRetry(req SendRequest) (*SendResponse, error) {
	log := s.log.WithField("sendReq", req)

	// Verify the request
	if err := req.Verify(); err != nil {
		log.WithError(err).Error("SendRequest.Verify failed")
		return nil, err
	}

	// This loop tries to send the coins until it succeeds.
	// TODO: if this gets stuck, nothing will proceed.
	// Add logic to give up sending after some number of retries if necessary
	// Most likely reason for send() to fail is because the skyd node
	// is unavailable.
	for {
		txid, err := s.skycli.Send(req.Address, req.Coins)
		if err != nil {
			log.WithError(err).Error("skycli.Send failed, trying again...")

			select {
			case <-s.quit:
				return nil, nil
			case <-time.After(sendCoinRetryWait):
			}

			continue
		}

		return &SendResponse{
			Txid: txid,
			Req:  req,
		}, nil
	}
}

// Shutdown close the sender
func (s *SendService) Shutdown() {
	close(s.quit)
}
