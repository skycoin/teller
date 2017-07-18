package monitor

import (
	"time"

	"github.com/skycoin/teller/src/logger"
)

// Option used as optional arguments when creating monitor instance
type Option func(*Monitor)

// EventBuffSize sets the monitor's event channel buffer size
func EventBuffSize(n int) Option {
	return func(m *Monitor) {
		m.eventBuffSize = n
	}
}

// PushTimeout sets the push timeout duration
func PushTimeout(d time.Duration) Option {
	return func(m *Monitor) {
		m.pushTimeout = d
	}
}

// CheckPeriod sets the monitor's balance checking period
func CheckPeriod(d time.Duration) Option {
	return func(m *Monitor) {
		m.checkPeriod = d
	}
}

// Logger sets the logger
func Logger(log logger.Logger) Option {
	return func(m *Monitor) {
		m.log = log
	}
}
