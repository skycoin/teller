package service

import (
	"errors"

	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/logger"
)

// gateway is used to limit the service's direct Export methods.
type gateway struct {
	s *Service
}

// Logger returns the service's logger
func (gw *gateway) Logger() logger.Logger {
	return gw.s.log
}

// ResetPongTimer resets the pong timer
func (gw *gateway) ResetPongTimer() {
	gw.s.strand(func() {
		gw.s.session.resetPongTimer(gw.s.cfg.PongTimeout)
	})
}

// AddMonitor add monitor of specific deposit address
func (gw *gateway) AddMonitor(mm *daemon.MonitorMessage) (err error) {
	if mm == nil {
		return errors.New("Nil daemon.MonitorMessage")
	}
	gw.s.strand(func() {
		cv := coinValue{
			Address:    mm.DepositCoin.Address,
			CoinName:   mm.DepositCoin.CoinName,
			ICOAddress: mm.ICOCoin.Address,
		}
		err = gw.s.exchange.AddMonitor(cv)
	})
	return
}

// GetExchangeLogs returns exchange logs whose id are in the given range
func (gw *gateway) GetExchangeLogs(start, end int) ([]daemon.ExchangeLog, error) {
	return gw.s.exchange.GetLogs(start, end)
}

func (gw *gateway) GetExchangeLogsLen() int {
	return gw.s.exchange.GetLogsLen()
}
