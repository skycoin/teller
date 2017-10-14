package sender

import (
	"errors"
)

var (
	// ErrSendBufferFull the send service's request channel is full
	ErrSendBufferFull = errors.New("send service's request queue is full")
	// ErrServiceClosed the sender has closed
	ErrServiceClosed = errors.New("send service closed")
)

// Sender provids apis for sending skycoin
type Sender interface {
	Send(destAddr string, coins uint64) *SendResponse
	IsTxConfirmed(txid string) *ConfirmResponse
}

// RetrySender provids helper function to send coins with send service
// All requests will retry until succeeding.
type RetrySender struct {
	s *SendService
}

// NewRetrySender creates new sender
func NewRetrySender(s *SendService) *RetrySender {
	return &RetrySender{
		s: s,
	}
}

// Send send coins to dest address in a goroutine
func (s *RetrySender) Send(destAddr string, coins uint64) *SendResponse {
	rspC := make(chan *SendResponse, 1)

	go func() {
		s.s.sendChan <- SendRequest{
			Address: destAddr,
			Coins:   coins,
			RspC:    rspC,
		}
	}()

	return <-rspC
}

// IsTxConfirmed checks if tx is confirmed
func (s *RetrySender) IsTxConfirmed(txid string) *ConfirmResponse {
	rspC := make(chan *ConfirmResponse, 1)

	go func() {
		s.s.confirmChan <- ConfirmRequest{
			Txid: txid,
			RspC: rspC,
		}
	}()

	return <-rspC
}
