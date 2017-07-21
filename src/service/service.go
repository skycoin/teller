package service

import (
	"context"
	"errors"
	"net"
	"path/filepath"

	"time"

	"os/user"

	"sync"

	"github.com/boltdb/bolt"
	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/logger"
)

const (
	// all things below are default values for service, use Option to change them.
	reconnectTime = 5 * time.Second
	pingTimeout   = 5 * time.Second
	pongTimeout   = 10 * time.Second
	dialTimeout   = 5 * time.Second

	scanPeriod = 30 * time.Second

	nodeRPCAddr = "127.0.0.1:7430"
	nodeWltFile = ".skycoin/wallets/skycoin.wlt"

	depositCoin = "bitcoin" // default deposit coin
	icoCoin     = "skycoin" // default ico coin
)

var (
	ErrPongTimout = errors.New("Pong message timeout")
)

// Service provides the ico service
type Service struct {
	log       logger.Logger
	cfg       config          // service configuration info
	session   *session        // connection is maintained in session, and when session done, means this connection is done.
	proxyAddr string          // ico proxy address, format in host:port
	auth      *daemon.Auth    // records the secrects for authentication,
	mux       *daemon.Mux     // used for dispatching message to corresponding handler
	cxt       context.Context // usually it's not recommand to record context as member variable, while in this case,
	// we have to record it here, so that Run method can check it to exist the service loop.
	reqc chan func() // reqeust function channel, requests go throught this channel can make the accessing of
	// member variable thread safe.
	exchange *exchange // exchange is in charge of monitoring deposit coin's balance, and send ico coins to given address.
	gateway  *gateway  // gateway will be used in message handlers, provides methods to access service's resource
	// in safe mode, and can also reduce the export methods number of service.
	wg sync.WaitGroup // for making sure that all goroutine are returned before service exit.
}

// config records the configurations of the service
type config struct {
	ReconnectTime time.Duration
	PingTimeout   time.Duration
	PongTimeout   time.Duration
	DialTimeout   time.Duration

	ScanPeriod time.Duration // scan period

	NodeRPCAddr  string // node's rpc address
	NodeWltFile  string // ico coin's wallet path
	ExchangeRate rateTable

	DepositCoin string // deposit coin name
	ICOCoin     string // ico coin name
}

// Cancel callback function for canceling the service.
type Cancel func()

// New creates a ico service
func New(proxyAddr string, auth *daemon.Auth, db *bolt.DB, ops ...Option) (*Service, Cancel) {
	cur, err := user.Current()
	if err != nil {
		panic(err)
	}
	cxt, cancel := context.WithCancel(context.Background())
	s := &Service{
		proxyAddr: proxyAddr,
		auth:      auth,
		cxt:       cxt,
		reqc:      make(chan func(), 1),
		log:       logger.NewLogger("", false),
		cfg: config{
			ReconnectTime: reconnectTime,
			PingTimeout:   pingTimeout,
			PongTimeout:   pongTimeout,
			DialTimeout:   dialTimeout,
			ScanPeriod:    scanPeriod,
			NodeRPCAddr:   nodeRPCAddr,
			NodeWltFile:   filepath.Join(cur.HomeDir, nodeWltFile),
			DepositCoin:   depositCoin,
			ICOCoin:       icoCoin,
		},
	}

	for _, op := range ops {
		op(s)
	}

	s.mux = daemon.NewMux(s.log)
	s.gateway = &gateway{s}

	cfg := exchgConfig{
		db:          db,
		log:         s.log,
		rateTable:   s.cfg.ExchangeRate,
		checkPeriod: s.cfg.ScanPeriod,
		nodeRPCAddr: s.cfg.NodeRPCAddr,
		nodeWltFile: s.cfg.NodeWltFile,
		depositCoin: s.cfg.DepositCoin,
		icoCoin:     s.cfg.ICOCoin,
	}

	s.exchange = newExchange(&cfg)

	bindHandlers(s)
	return s, func() {
		cancel()
		s.wg.Wait()
	}
}

// Run starts the service
func (s *Service) Run() {
	s.wg.Add(1)
	go func() {
		defer func() {
			s.wg.Done()
			s.log.Println("Service exit")
		}()

		// start the btc monitor
		exchgErrC := s.exchange.Run(s.cxt)

		errC := make(chan error, 1)
		s.newSession(errC)
		for {
			select {
			case <-s.cxt.Done():
				s.log.Debug(s.cxt.Err())
				s.closeSession()
				return
			case err := <-exchgErrC:
				s.log.Debugln("Exchange error:", err)
				return
			case err := <-errC:
				s.log.Debugln("Session error:", err)
				time.AfterFunc(s.cfg.ReconnectTime, func() {
					s.newSession(errC)
				})
				continue
			case req := <-s.reqc:
				req()
			}
		}

	}()
}

// HandleFunc adds handler for given message type to mux
func (s *Service) HandleFunc(tp daemon.MsgType, h daemon.Handler) {
	s.mux.HandleFunc(tp, h)
}

func (s *Service) newSession(errC chan error) {
	s.wg.Add(1)
	s.log.Debugln("New session")
	go func() {
		defer func() {
			s.wg.Done()
			s.log.Debugln("Session closed")
		}()
		s.log.Println("Connect to", s.proxyAddr)
		conn, err := net.DialTimeout("tcp", s.proxyAddr, s.cfg.DialTimeout)
		if err != nil {
			errC <- err
			return
		}

		sn, err := daemon.NewSession(conn, s.auth, s.mux, true, daemon.Logger(s.log))
		if err != nil {
			errC <- err
			return
		}

		s.session = &session{
			pingTicker: time.NewTicker(s.cfg.PingTimeout),
			pongTimer:  time.NewTimer(s.cfg.PongTimeout),
			Session:    sn,
		}
		ec := make(chan error, 1)
		go func() {
			ec <- s.session.Run(s.cxt)
		}()

		// send ping message
		s.sendPing()
		for {
			select {
			case e := <-ec:
				errC <- e
				return
			case <-s.session.pongTimer.C:
				s.log.Debugln("Pong message time out")
				s.session.close()
				errC <- ErrPongTimout
				return
			case <-s.session.pingTicker.C:
				// send ping message
				s.log.Debugln("Send ping message")
				s.sendPing()
			}
		}
	}()
	return
}

func (s *Service) closeSession() {
	if s.session != nil {
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
