package teller

import (
	"errors"

	"github.com/sirupsen/logrus"

	"github.com/skycoin/teller/src/addrs"
	"github.com/skycoin/teller/src/config"
	"github.com/skycoin/teller/src/exchange"
	"github.com/skycoin/teller/src/scanner"
)

var (
	// ErrMaxBoundAddresses is returned when the maximum number of address to bind to a SKY address has been reached
	ErrMaxBoundAddresses = errors.New("The maximum number of BTC addresses have been assigned to this SKY address")
)

// Teller provides the HTTP and teller service
type Teller struct {
	cfg      config.Teller
	log      logrus.FieldLogger
	httpServ *HTTPServer // HTTP API
	quit     chan struct{}
	done     chan struct{}
}

// New creates a Teller
func New(log logrus.FieldLogger, exchanger exchange.Exchanger, addrGen, ethAddrGen addrs.AddrGenerator, cfg config.Config) *Teller {
	return &Teller{
		cfg:  cfg.Teller,
		log:  log.WithField("prefix", "teller"),
		quit: make(chan struct{}),
		done: make(chan struct{}),
		httpServ: NewHTTPServer(log, cfg.Redacted(), &Service{
			cfg:        cfg.Teller,
			exchanger:  exchanger,
			addrGen:    addrGen,
			ethAddrGen: ethAddrGen,
		}),
	}
}

// Run starts the Teller
func (s *Teller) Run() error {
	log := s.log.WithField("config", s.cfg)
	log.Info("Starting teller...")
	defer log.Info("Teller closed")
	defer close(s.done)

	if err := s.httpServ.Run(); err != nil {
		log.WithError(err).Error(err)
		select {
		case <-s.quit:
			return nil
		default:
			return err
		}
	}

	return nil
}

// Shutdown close the Teller
func (s *Teller) Shutdown() {
	s.log.Info("Shutting down teller service")
	defer s.log.Info("Shutdown teller service")

	close(s.quit)
	s.httpServ.Shutdown()
	<-s.done
}

// Service combines Exchanger and AddrGenerator
type Service struct {
	cfg        config.Teller
	exchanger  exchange.Exchanger  // exchange Teller client
	addrGen    addrs.AddrGenerator // address generator
	ethAddrGen addrs.AddrGenerator // address generator
}

// BindAddress binds skycoin address with a deposit btc address
// return btc address
// TODO -- support multiple coin types
func (s *Service) BindAddress(skyAddr, coinType string) (string, error) {
	if s.cfg.MaxBoundBtcAddresses > 0 {
		num, err := s.exchanger.GetBindNum(skyAddr)
		if err != nil {
			return "", err
		}

		if num >= s.cfg.MaxBoundBtcAddresses {
			return "", ErrMaxBoundAddresses
		}
	}
	switch coinType {
	case scanner.CoinTypeBTC:

		btcAddr, err := s.addrGen.NewAddress()
		if err != nil {
			return "", err
		}

		//btcStoreAddr := dbutil.Join(coinType, btcAddr, ":")
		if err := s.exchanger.BindAddress(skyAddr, btcAddr); err != nil {
			return "", err
		}
		return btcAddr, nil
	case scanner.CoinTypeETH:

		ethAddr, err := s.ethAddrGen.NewAddress()
		if err != nil {
			return "", err
		}

		//ethStoreAddr := dbutil.Join(coinType, ethAddr, ":")
		if err := s.exchanger.BindAddress(skyAddr, ethAddr); err != nil {
			return "", err
		}
		return ethAddr, nil
	default:
		return "", errors.New("unsupport coinType")
	}
}

// GetDepositStatuses returns deposit status of given skycoin address
func (s *Service) GetDepositStatuses(skyAddr string) ([]exchange.DepositStatus, error) {
	return s.exchanger.GetDepositStatuses(skyAddr)
}
