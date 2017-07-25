package service

import (
	"github.com/skycoin/teller/src/logger"
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
	btcAddr, err := gw.s.btcAddrGen.NewAddress()
	if err != nil {
		return "", err
	}

	if err := gw.s.excli.BindAddress(btcAddr, skyAddr); err != nil {
		return "", err
	}

	return btcAddr, nil
}
