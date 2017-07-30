package service

import (
	"errors"
	"net"

	"time"

	"io"

	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/logger"
)

const (
	// all things below are default values for service, use Option to change them.
	reconnectTime = 5 * time.Second
	pingTimeout   = 5 * time.Second
	pongTimeout   = 10 * time.Second
	dialTimeout   = 5 * time.Second
)

var (
	// ErrPongTimeout not receive pong message in given time error
	ErrPongTimout = errors.New("pong message timeout")
)

// BtcAddrGenerator generate new deposit address
type BtcAddrGenerator interface {
	NewAddress() (string, error)
}

// Exchanger provids apis to interact with exchange service
type Exchanger interface {
	BindAddress(btcAddr, skyAddr string) error
	GetDepositStatuses(skyAddr string) ([]daemon.DepositStatus, error)
}

// Service provides the ico service
type Service struct {
	logger.Logger
	cfg     Config      // service configuration info
	session *session    // connection is maintained in session, and when session done, means this connection is done.
	mux     *daemon.Mux // used for dispatching message to corresponding handler
	auth    *daemon.Auth

	excli      Exchanger        // exchange service client
	btcAddrGen BtcAddrGenerator // btc address generator
	gateway    *gateway         // gateway will be used in message handlers, provides methods to access service's resource
	reqc       chan func()      // reqeust function channel, to queue the variable update request
	quit       chan struct{}
}

// Config records the configurations of the service
type Config struct {
	ProxyAddr string

	ReconnectTime time.Duration
	PingTimeout   time.Duration
	PongTimeout   time.Duration
	DialTimeout   time.Duration
}

// New creates a ico service
func New(cfg Config, auth *daemon.Auth, log logger.Logger, excli Exchanger, btcAddrGen BtcAddrGenerator) *Service {
	if cfg.ReconnectTime == 0 {
		cfg.ReconnectTime = reconnectTime
	}

	if cfg.PingTimeout == 0 {
		cfg.PingTimeout = pingTimeout
	}

	if cfg.PongTimeout == 0 {
		cfg.PongTimeout = pongTimeout
	}

	if cfg.DialTimeout == 0 {
		cfg.DialTimeout = dialTimeout
	}

	s := &Service{
		cfg:        cfg,
		Logger:     log,
		auth:       auth,
		excli:      excli,
		btcAddrGen: btcAddrGen,
		reqc:       make(chan func(), 1),
		quit:       make(chan struct{}),
	}

	s.mux = daemon.NewMux(s.Logger)

	s.gateway = &gateway{
		Logger: s.Logger,
		s:      s,
	}

	// bind message handlers
	bindHandlers(s)

	return s
}

// Run starts the service
func (s *Service) Run() error {
	s.Println("Start teller service...")
	defer s.Println("Teller Service closed")

	for {
		if err := s.newSession(); err != nil {
			switch err {
			case io.EOF:
				s.Println("Proxy connection break..")
			case daemon.ErrAuth:
				return err
			default:
				s.Println(err)
			}
		}
		select {
		case <-s.quit:
			return nil
		case <-time.After(s.cfg.ReconnectTime):
			continue
		}
	}
}

// Shutdown close the service
func (s *Service) Shutdown() {
	close(s.quit)
	s.closeSession()
}

// HandleFunc adds handler for given message type to mux
func (s *Service) HandleFunc(tp daemon.MsgType, h daemon.Handler) {
	s.mux.HandleFunc(tp, h)
}

func (s *Service) newSession() error {
	s.Debugln("New session")

	defer s.Debugln("Session closed")
	s.Println("Connect to proxy address", s.cfg.ProxyAddr)

	conn, err := net.DialTimeout("tcp", s.cfg.ProxyAddr, s.cfg.DialTimeout)
	if err != nil {
		return err
	}

	s.Println("Connect success")

	sn, err := daemon.NewSession(conn, s.auth, s.mux, true, daemon.Logger(s.Logger))
	if err != nil {
		return err
	}

	s.session = &session{
		pingTicker: time.NewTicker(s.cfg.PingTimeout),
		pongTimer:  time.NewTimer(s.cfg.PongTimeout),
		Session:    sn,
	}

	errC := make(chan error, 1)
	go func() {
		errC <- s.session.Run()
	}()

	// send ping message
	s.sendPing()

	for {
		select {
		case err := <-errC:
			return err
		case <-s.session.pongTimer.C:
			s.Debugln("Pong message time out")
			s.session.close()
			s.session = nil
			return ErrPongTimout
		case <-s.session.pingTicker.C:
			// send ping message
			s.Debugln("Send ping message")
			s.sendPing()
		case req := <-s.reqc:
			req()
		}
	}
}

func (s *Service) closeSession() {
	if s.session != nil && !s.session.isClosed() {
		s.session.close()
	}
}

func (s *Service) strand(f func()) {
	q := make(chan struct{})
	s.reqc <- func() {
		defer close(q)
		f()
	}
	<-q
}

func (s *Service) sendPing() {
	s.session.Write(&daemon.PingMessage{Value: "PING"})
}
