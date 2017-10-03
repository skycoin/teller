package service

import (
	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/util/logger"
)

// gateway is used to limit the service's direct Export methods.
type gateway struct {
	logger.Logger
	s *Service
}

// ResetPongTimer resets the pong timer
func (gw *gateway) ResetPongTimer() {
	gw.s.strand(func() {
		gw.s.session.resetPongTimer(gw.s.cfg.PongTimeout)
	})
}

// BindAddress binds skycoin address with a deposit btc address
// return btc address
func (gw *gateway) BindAddress(skyAddr string) (string, error) {
	if gw.s.cfg.MaxBind != 0 {
		num, err := gw.s.excli.BindNum(skyAddr)
		if err != nil {
			return "", err
		}

		if num >= gw.s.cfg.MaxBind {
			return "", ErrMaxBind
		}
	}

	btcAddr, err := gw.s.btcAddrGen.NewAddress()
	if err != nil {
		return "", err
	}

	if err := gw.s.excli.BindAddress(btcAddr, skyAddr); err != nil {
		return "", err
	}

	return btcAddr, nil
}

// GetDepositStatuses returns deposit status of given skycoin address
func (gw *gateway) GetDepositStatuses(skyAddr string) ([]daemon.DepositStatus, error) {
	return gw.s.excli.GetDepositStatuses(skyAddr)
}
