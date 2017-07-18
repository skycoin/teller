package service

import (
	"time"

	"github.com/skycoin/teller/src/logger"
	gconfig "github.com/skycoin/teller/src/service/config"
)

// Option represents the service option
type Option func(*Service)

// ReconnectTime sets reconnect time config
func ReconnectTime(d time.Duration) Option {
	return func(s *Service) {
		s.cfg.ReconnectTime = d
	}
}

// PingTimeout sets ping timeout duration
func PingTimeout(d time.Duration) Option {
	return func(s *Service) {
		s.cfg.PingTimeout = d
	}
}

// PongTimeout sets pong timeout duration
func PongTimeout(d time.Duration) Option {
	return func(s *Service) {
		s.cfg.PongTimeout = d
	}
}

// DialTimeout sets dial timeout duration
func DialTimeout(d time.Duration) Option {
	return func(s *Service) {
		s.cfg.DialTimeout = d
	}
}

// NodeRPCAddr sets the node rpc address
func NodeRPCAddr(addr string) Option {
	return func(s *Service) {
		s.cfg.NodeRPCAddr = addr
	}
}

// NodeWalletPath sets the node wallet file path
func NodeWalletPath(path string) Option {
	return func(s *Service) {
		s.cfg.NodeWltFile = path
	}
}

// Logger sets the logger
func Logger(log logger.Logger) Option {
	return func(s *Service) {
		s.log = log
	}
}

// MonitorCheckPeriod sets the monitor check period
func MonitorCheckPeriod(d time.Duration) Option {
	return func(s *Service) {
		s.cfg.MonitorCheckPeriod = d
	}
}

// ExchangeRateTable sets exchange rate
func ExchangeRateTable(rateTable []gconfig.ExchangeRate) Option {
	return func(s *Service) {
		s.cfg.ExchangeRate.loadFromConfig(rateTable)
	}
}

// DepositCoin sets deposit coin name
func DepositCoin(coin string) Option {
	return func(s *Service) {
		s.cfg.DepositCoin = coin
	}
}

// ICOCoin sets ico coin name
func ICOCoin(coin string) Option {
	return func(s *Service) {
		s.cfg.ICOCoin = coin
	}
}
