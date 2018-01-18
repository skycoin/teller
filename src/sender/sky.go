// Package sender provids send service for skycoin
package sender

import (
	"errors"

	"time"

	"github.com/sirupsen/logrus"

	"github.com/skycoin/skycoin/src/api/cli"
	"github.com/skycoin/skycoin/src/api/webrpc"
	"github.com/skycoin/skycoin/src/coin"
)

const (
	broadcastTxRetryWait = 3 * time.Second
	confirmTxRetryWait   = 3 * time.Second
)

// BroadcastTxRequest send coin request struct
type BroadcastTxRequest struct {
	Tx   *coin.Transaction
	RspC chan *BroadcastTxResponse // response
}

// Verify verifies the request parameters
func (r BroadcastTxRequest) Verify() error {
	if r.Tx == nil {
		return errors.New("Tx empty")
	}

	return nil
}

// BroadcastTxResponse send response
type BroadcastTxResponse struct {
	Txid string
	Err  error
	Req  BroadcastTxRequest
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
	log             logrus.FieldLogger
	SkyClient       SkyClient
	quit            chan struct{}
	done            chan struct{}
	broadcastTxChan chan BroadcastTxRequest
	confirmChan     chan ConfirmRequest
}

// SkyClient defines a Skycoin RPC client interface for sending and confirming
type SkyClient interface {
	CreateTransaction(string, uint64) (*coin.Transaction, error)
	BroadcastTransaction(*coin.Transaction) (string, error)
	GetTransaction(string) (*webrpc.TxnResult, error)
	Balance() (*cli.Balance, error)
}

// NewService creates sender instance
func NewService(log logrus.FieldLogger, skycli SkyClient) *SendService {
	return &SendService{
		SkyClient:       skycli,
		log:             log.WithField("prefix", "sender.service"),
		quit:            make(chan struct{}),
		done:            make(chan struct{}),
		broadcastTxChan: make(chan BroadcastTxRequest, 10),
		confirmChan:     make(chan ConfirmRequest, 10),
	}
}

// Run start the send service
func (s *SendService) Run() error {
	log := s.log
	log.Info("Start skycoin send service")
	defer log.Info("Skycoin send service closed")
	defer close(s.done)

	for {
		select {
		case <-s.quit:
			return nil
		case req := <-s.broadcastTxChan:
			rsp, err := s.BroadcastTxRetry(req)

			if err != nil {
				log.WithError(err).Error("BroadcastTxRetry failed")
				rsp = &BroadcastTxResponse{
					Req: req,
					Err: err,
				}
			}

			select {
			case req.RspC <- rsp:
			case <-s.quit:
				return nil
			}
		case req := <-s.confirmChan:
			rsp, err := s.ConfirmRetry(req)

			if err != nil {
				log.WithError(err).Error("ConfirmRetry failed")
				rsp = &ConfirmResponse{
					Req: req,
					Err: err,
				}
			}

			select {
			case req.RspC <- rsp:
			case <-s.quit:
				return nil
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

	tx, err := s.SkyClient.GetTransaction(req.Txid)
	if err != nil {
		log.WithError(err).Error("SkyClient.GetTransaction failed")
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
		tx, err := s.SkyClient.GetTransaction(req.Txid)
		if err != nil {
			log.WithError(err).Error("SkyClient.GetTransaction failed, trying again...")

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

// BroadcastTx sends coins
func (s *SendService) BroadcastTx(req BroadcastTxRequest) (*BroadcastTxResponse, error) {
	log := s.log.WithField("broadcastTxTxid", req.Tx.TxIDHex())

	// Verify the request
	if err := req.Verify(); err != nil {
		log.WithError(err).Error("BroadcastTxRequest.Verify failed")
		return nil, err
	}

	txid, err := s.SkyClient.BroadcastTransaction(req.Tx)
	if err != nil {
		log.WithError(err).Error("SkyClient.BroadcastTransaction failed")
		return nil, err
	}

	return &BroadcastTxResponse{
		Txid: txid,
		Req:  req,
	}, nil
}

// BroadcastTxRetry sends coins and will retry indefinitely until it succeeds
func (s *SendService) BroadcastTxRetry(req BroadcastTxRequest) (*BroadcastTxResponse, error) {
	log := s.log.WithField("broadcastTxTxid", req.Tx.TxIDHex())

	// Verify the request
	if err := req.Verify(); err != nil {
		log.WithError(err).Error("BroadcastTxRequest.Verify failed")
		return nil, err
	}

	// This loop tries to send the coins until it succeeds.
	// TODO: if this gets stuck, nothing will proceed.
	// Add logic to give up sending after some number of retries if necessary
	// Most likely reason for send() to fail is because the skyd node
	// is unavailable.
	for {
		txid, err := s.SkyClient.BroadcastTransaction(req.Tx)
		if err != nil {
			log.WithError(err).Error("SkyClient.BroadcastTransaction failed, trying again...")

			select {
			case <-s.quit:
				return nil, nil
			case <-time.After(broadcastTxRetryWait):
			}

			continue
		}

		return &BroadcastTxResponse{
			Txid: txid,
			Req:  req,
		}, nil
	}
}

// Shutdown close the sender
func (s *SendService) Shutdown() {
	close(s.quit)
	<-s.done
}
