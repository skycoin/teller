package service

import (
	"sync"
	"time"

	"github.com/skycoin/teller/src/daemon"
)

type session struct {
	sync.Mutex
	closed bool
	*daemon.Session
	pingTicker *time.Ticker // ping ticker for sending ping message periodically
	pongTimer  *time.Timer  // pong timer used to check if the service can receive pong message in specific duration
}

func (s *session) close() {
	s.Lock()
	s.closed = true
	s.Unlock()

	if s.Session != nil {
		s.Close()
	}

	if s.pingTicker != nil {
		s.pingTicker.Stop()
		s.pingTicker = nil
	}

	if s.pongTimer != nil {
		s.pongTimer.Stop()
		s.pongTimer = nil
	}
}

func (s *session) isClosed() bool {
	s.Lock()
	defer s.Unlock()
	return s.closed
}

func (s *session) resetPongTimer(d time.Duration) {
	if s.pongTimer != nil {
		s.pongTimer.Reset(d)
	}
}
