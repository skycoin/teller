// Package sender provids send service for mdl
package sender

import (
	"errors"

	"time"

	"github.com/sirupsen/logrus"

	"github.com/MDLlife/MDL/src/api/cli"
	"github.com/MDLlife/MDL/src/api/webrpc"
	"github.com/MDLlife/MDL/src/coin"
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

// SendService is in charge of sending mdl
type SendService struct {
	log             logrus.FieldLogger
	MDLClient       MDLClient
	quit            chan struct{}
	done            chan struct{}
	broadcastTxChan chan BroadcastTxRequest
	confirmChan     chan ConfirmRequest
}

// MDLClient defines a MDL RPC client interface for sending and confirming
type MDLClient interface {
	CreateTransaction(string, uint64) (*coin.Transaction, error)
	BroadcastTransaction(*coin.Transaction) (string, error)
	GetTransaction(string) (*webrpc.TxnResult, error)
	Balance() (*cli.Balance, error)
}

// NewService creates sender instance
func NewService(log logrus.FieldLogger, mdlcli MDLClient) *SendService {
	return &SendService{
		MDLClient:       mdlcli,
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
	log.Info("Start mdl send service")
	defer log.Info("MDL send service closed")
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

	tx, err := s.MDLClient.GetTransaction(req.Txid)
	if err != nil {
		log.WithError(err).Error("MDLClient.GetTransaction failed")
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
	// Most likely reason for GetTransaction() to fail is because the mdld node
	// is unavailable.
	for {
		tx, err := s.MDLClient.GetTransaction(req.Txid)
		if err != nil {
			log.WithError(err).Error("MDLClient.GetTransaction failed, trying again...")

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

	txid, err := s.MDLClient.BroadcastTransaction(req.Tx)
	if err != nil {
		log.WithError(err).Error("MDLClient.BroadcastTransaction failed")
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
	// Most likely reason for send() to fail is because the mdld node
	// is unavailable.
	for {
		txid, err := s.MDLClient.BroadcastTransaction(req.Tx)
		if err != nil {
			log.WithError(err).Error("MDLClient.BroadcastTransaction failed, trying again...")

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
