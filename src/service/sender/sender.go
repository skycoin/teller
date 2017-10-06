package sender

import (
	"errors"
	"time"
)

// ErrSendBufferFull the send service's request channel is full
var (
	ErrSendBufferFull = errors.New("send service's request queue is full")
	ErrServiceClosed  = errors.New("send service closed")
)

// Sender provids helper function to send coins with send service
type Sender struct {
	s *SendService
}

// NewSender creates new sender
func NewSender(s *SendService) *Sender {
	return &Sender{
		s: s,
	}
}

// SendOption send option struct
type SendOption struct {
	Timeout time.Duration
}

// SendAsync send coins to dest address in a goroutine
func (s *Sender) SendAsync(destAddr string, coins uint64) <-chan Response {
	rspC := make(chan Response, 1)

	go func() {
		s.s.reqChan <- Request{
			Address: destAddr,
			Coins:   coins,
			RspC:    rspC,
		}
	}()

	return rspC
}

// IsClosed checks if the service is closed
func (s *Sender) IsClosed() bool {
	return s.s.IsClosed()
}

// IsTxConfirmed checks if tx is confirmed
func (s *Sender) IsTxConfirmed(txid string) bool {
	tx, err := s.s.skycli.GetTransaction(txid)
	if err != nil {
		return false
	}

	return tx.Transaction.Status.Confirmed
}
