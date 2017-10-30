package teller

import (
	"errors"

	"github.com/sirupsen/logrus"

	"github.com/skycoin/teller/src/addrs"
	"github.com/skycoin/teller/src/exchange"
)

var (
	// ErrMaxBind is returned when the maximum number of address to bind to a SKY address has been reached
	ErrMaxBind = errors.New("max bind reached")
)

// Config configures Teller
type Config struct {
	Service ServiceConfig
	HTTP    HTTPConfig
}

// Teller provides the HTTP and teller service
type Teller struct {
	log logrus.FieldLogger
	cfg Config // Teller configuration info

	httpServ *httpServer // HTTP API

	quit chan struct{}
}

// New creates a Teller
func New(log logrus.FieldLogger, exchanger exchange.Exchanger, addrGen addrs.AddrGenerator, cfg Config) (*Teller, error) {
	if err := cfg.HTTP.Validate(); err != nil {
		return nil, err
	}

	return &Teller{
		cfg:  cfg,
		log:  log.WithField("prefix", "teller"),
		quit: make(chan struct{}),
		httpServ: newHTTPServer(log, cfg.HTTP, &service{
			cfg:       cfg.Service,
			exchanger: exchanger,
			addrGen:   addrGen,
		}),
	}, nil
}

// Run starts the Teller
func (s *Teller) Run() error {
	s.log.Info("Starting teller...")
	defer s.log.Info("Teller closed")

	if err := s.httpServ.Run(); err != nil {
		s.log.WithError(err).Error()
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
	close(s.quit)
	s.httpServ.Shutdown()
}

// ServiceConfig configures service
type ServiceConfig struct {
	MaxBind int // maximum number of addresses allowed to bind to a SKY address
}

// service combines Exchanger and AddrGenerator
type service struct {
	cfg       ServiceConfig
	exchanger exchange.Exchanger  // exchange Teller client
	addrGen   addrs.AddrGenerator // address generator
}

// BindAddress binds skycoin address with a deposit btc address
// return btc address
// TODO -- support multiple coin types
func (s *service) BindAddress(skyAddr string) (string, error) {
	if s.cfg.MaxBind != 0 {
		num, err := s.exchanger.GetBindNum(skyAddr)
		if err != nil {
			return "", err
		}

		if num >= s.cfg.MaxBind {
			return "", ErrMaxBind
		}
	}

	btcAddr, err := s.addrGen.NewAddress()
	if err != nil {
		return "", err
	}

	if err := s.exchanger.BindAddress(skyAddr, btcAddr); err != nil {
		return "", err
	}

	return btcAddr, nil
}

// GetDepositStatuses returns deposit status of given skycoin address
func (s *service) GetDepositStatuses(skyAddr string) ([]exchange.DepositStatus, error) {
	return s.exchanger.GetDepositStatuses(skyAddr)
}
