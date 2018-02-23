package teller

import (
	"errors"

	"github.com/sirupsen/logrus"

	"github.com/skycoin/teller/src/addrs"
	"github.com/skycoin/teller/src/config"
	"github.com/skycoin/teller/src/exchange"
)

var (
	// ErrMaxBoundAddresses is returned when the maximum number of address to bind to a SKY address has been reached
	ErrMaxBoundAddresses = errors.New("The maximum number of addresses have been assigned to this SKY address")
	// ErrBindDisabled is returned if address binding is disabled
	ErrBindDisabled = errors.New("Address binding is disabled")
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
func New(log logrus.FieldLogger, exchanger exchange.Exchanger, addrManager *addrs.AddrManager, cfg config.Config) *Teller {
	return &Teller{
		cfg:  cfg.Teller,
		log:  log.WithField("prefix", "teller"),
		quit: make(chan struct{}),
		done: make(chan struct{}),
		httpServ: NewHTTPServer(log, cfg.Redacted(), &Service{
			cfg:         cfg.Teller,
			exchanger:   exchanger,
			addrManager: addrManager,
		}, exchanger),
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
	cfg         config.Teller
	exchanger   exchange.Exchanger // exchange Teller client
	addrManager *addrs.AddrManager // address manager
}

// BindAddress binds skycoin address with a deposit address according to coinType
// return deposit address
func (s *Service) BindAddress(skyAddr, coinType string) (*exchange.BoundAddress, error) {
	if !s.cfg.BindEnabled {
		return nil, ErrBindDisabled
	}

	if s.cfg.MaxBoundAddresses > 0 {
		num, err := s.exchanger.GetBindNum(skyAddr)
		if err != nil {
			return nil, err
		}

		if num >= s.cfg.MaxBoundAddresses {
			return nil, ErrMaxBoundAddresses
		}
	}

	depositAddr, err := s.addrManager.NewAddress(coinType)
	if err != nil {
		return nil, err
	}

	return s.exchanger.BindAddress(skyAddr, depositAddr, coinType)
}

// GetDepositStatuses returns deposit status of given skycoin address
func (s *Service) GetDepositStatuses(skyAddr string) ([]exchange.DepositStatus, error) {
	return s.exchanger.GetDepositStatuses(skyAddr)
}
