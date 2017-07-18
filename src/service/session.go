package service

import (
	"time"

	"github.com/skycoin/teller/src/daemon"
)

type session struct {
	*daemon.Session
	pingTicker *time.Ticker // ping ticker for sending ping message periodically
	pongTimer  *time.Timer  // pong timer used to check if the service can receive pong message in specific duration
}

func (s *session) close() {
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

func (s *session) resetPongTimer(d time.Duration) {
	if s.pongTimer != nil {
		s.pongTimer.Reset(d)
	}
}
