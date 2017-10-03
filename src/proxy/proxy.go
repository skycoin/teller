// Package proxy is the service run in the public server, and provides
// http apis for web server. The proxy use tcp socket to communicate with
// client, and all data are encrypted by ECDH and chacha20.
package proxy

import (
	"context"
	"errors"
	"net"
	"sync"

	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/util/logger"

	"io"
	"time"
)

const (
	pingTimeout = 10 * time.Second
)

// Proxy represents the ico proxy server
type Proxy struct {
	logger.Logger
	srvAddr       string // listen address, eg: 0.0.0.0:12345
	httpSrvAddr   string
	withoutTeller bool
	ln            net.Listener
	quit          chan struct{}
	sn            *daemon.Session
	connC         chan net.Conn
	auth          *daemon.Auth
	mux           *daemon.Mux
	reqC          chan func()
	pingTimer     *time.Timer

	httpServ *httpServ
	sync.Mutex
}

// Config proxy config
type Config struct {
	SrvAddr       string
	HTTPSrvAddr   string
	HTMLInterface bool
	HTMLStaticDir string
	StartAt       time.Time
	// If HTTPSSrvAddr is non-empty, either TlsHost must be set, or both TLSCert and TLSKey must be set
	// If TlsHost is set then TLSCert and TLSKey must not be set, and vice versa
	HTTPSSrvAddr string
	AutoTLSHost  string
	TLSCert      string
	TLSKey       string
	Throttle     Throttle

	WithoutTeller bool
}

// New creates proxy instance
func New(cfg Config, auth *daemon.Auth, ops ...Option) *Proxy {
	if auth == nil {
		panic("Auth is nil")
	}

	if cfg.HTTPSrvAddr == "" && cfg.HTTPSSrvAddr == "" {
		panic("at least one of -http-service-addr, -https-service-addr must be set")
	}

	if cfg.HTTPSSrvAddr != "" && cfg.AutoTLSHost == "" && (cfg.TLSCert == "" || cfg.TLSKey == "") {
		panic("when using -tls, either -auto-tls-host or both -tls-cert and -tls-key must be set")
	}

	if (cfg.TLSCert == "" && cfg.TLSKey != "") || (cfg.TLSCert != "" && cfg.TLSKey == "") {
		panic("-tls-cert and -tls-key must be set or unset together")
	}

	if cfg.AutoTLSHost != "" && (cfg.TLSKey != "" || cfg.TLSCert != "") {
		panic("either use -auto-tls-host or both -tls-key and -tls-cert")
	}

	if cfg.HTTPSSrvAddr == "" && (cfg.AutoTLSHost != "" || cfg.TLSKey != "" || cfg.TLSCert != "") {
		panic("-auto-tls-host or -tls-key or -tls-cert is set but -tls is not enabled")
	}

	px := &Proxy{
		// default logger does not turn on debug mode, can use Logger option to set it.
		Logger:        logger.NewLogger("", false),
		srvAddr:       cfg.SrvAddr,
		httpSrvAddr:   cfg.HTTPSrvAddr,
		withoutTeller: cfg.WithoutTeller,
		connC:         make(chan net.Conn),
		auth:          auth,
		reqC:          make(chan func()),
		quit:          make(chan struct{}),
	}

	for _, op := range ops {
		op(px)
	}

	px.mux = daemon.NewMux(px.Logger)

	bindHandlers(px)

	gw := &gateway{
		p:      px,
		Logger: px.Logger,
	}

	px.httpServ = &httpServ{
		Logger:        px.Logger,
		Addr:          cfg.HTTPSrvAddr,
		StaticDir:     cfg.HTMLStaticDir,
		HTMLInterface: cfg.HTMLInterface,
		StartAt:       cfg.StartAt,
		HTTPSAddr:     cfg.HTTPSSrvAddr,
		AutoTLSHost:   cfg.AutoTLSHost,
		TLSCert:       cfg.TLSCert,
		TLSKey:        cfg.TLSKey,
		Gateway:       gw,
		Throttle:      cfg.Throttle,
		WithoutTeller: cfg.WithoutTeller,
	}

	return px
}

// Run start the proxy
func (px *Proxy) Run() error {
	var wg sync.WaitGroup
	errC := make(chan error, 1)
	if !px.withoutTeller {
		var err error
		px.ln, err = net.Listen("tcp", px.srvAddr)
		if err != nil {
			return err
		}

		px.Println("Proxy start, serve on", px.srvAddr)
		defer px.Println("Proxy service closed")

		// start connection handler process
		wg.Add(1)
		go func() {
			defer wg.Done()
			px.handleConnection()
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				conn, err := px.ln.Accept()
				if err != nil {
					select {
					case <-px.quit:
						return
					default:
						px.Println("Accept error:", err)
						continue
					}
				}

				select {
				case <-time.After(1 * time.Second):
					px.Printf("Close connection:%s, only one connection is allowed\n", conn.RemoteAddr())
					conn.Close()
				case px.connC <- conn:
				}
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case req := <-px.reqC:
					req()
				case <-px.quit:
					return
				}
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := px.httpServ.Run(); err != nil {
			select {
			case <-px.quit:
				return
			default:
				errC <- err
			}
		}
	}()

	done := make(chan struct{})

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case err := <-errC:
		return err
	}

}

// Shutdown close the proxy service
func (px *Proxy) Shutdown() {
	close(px.quit)

	if px.ln != nil {
		px.ln.Close()
		px.ln = nil
	}

	px.closeSession()

	if px.httpServ != nil {
		px.httpServ.Shutdown()
	}
}

func (px *Proxy) handleConnection() {
	execFuncC := make(chan func(conn net.Conn), 1)
	execFuncC <- px.newSession
	for {
		select {
		case <-px.quit:
			return
		case conn := <-px.connC:
			select {
			case <-time.After(2 * time.Second):
				px.Printf("Close connection %s, only one connection is allowed", conn.RemoteAddr())
				conn.Close()
				return
			case exec := <-execFuncC:
				exec(conn)
				select {
				case <-px.quit:
					return
				default:
					execFuncC <- exec
				}
			}
		}
	}
}

func (px *Proxy) newSession(conn net.Conn) {
	px.Debugln("New session")
	defer px.Debugln("Session closed")
	sn, err := daemon.NewSession(conn, px.auth, px.mux, false, daemon.Logger(px.Logger))
	if err != nil {
		px.Println(err)
		return
	}

	px.setSession(sn)

	px.pingTimer = time.NewTimer(pingTimeout)
	errC := make(chan error, 1)

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		errC <- sn.Run()
	}()

	select {
	case err := <-errC:
		if err != io.EOF && err != nil {
			px.Println(err)
		}
	case <-px.pingTimer.C:
		conn.Close()
	}
	wg.Wait()

	px.setSession(nil)
}

func (px *Proxy) strand(f func()) {
	q := make(chan struct{})
	px.reqC <- func() {
		defer close(q)
		f()
	}
	<-q
}

func (px *Proxy) write(m daemon.Messager) (err error) {
	px.Lock()
	defer px.Unlock()
	if px.sn == nil {
		err = errors.New("write failed, session is nil")
	}

	px.sn.Write(m)

	return
}

func (px *Proxy) writeWithContext(cxt context.Context, m daemon.Messager) error {
	px.Lock()
	defer px.Unlock()
	if px.sn == nil {
		return errors.New("write failed, session is nil")
	}

	return px.sn.WriteWithContext(cxt, m)
}

type closeStream func()

// openStream
func (px *Proxy) openStream(f func(daemon.Messager)) (int, closeStream, error) {
	px.Lock()
	defer px.Unlock()
	if px.sn == nil {
		return 0, func() {}, errors.New("session is nil")
	}

	id := px.sn.Sub(f)
	px.Debugln("Open stream:", id)
	cf := func() {
		defer px.Debugln("Close stream:", id)
		px.Lock()
		if px.sn != nil {
			px.sn.Unsub(id)
		}
		px.Unlock()
	}

	return id, cf, nil
}

func (px *Proxy) setSession(sn *daemon.Session) {
	px.Lock()
	px.sn = sn
	px.Unlock()
}

func (px *Proxy) closeSession() {
	px.Lock()
	if px.sn != nil {
		px.sn.Close()
	}
	px.Unlock()
}

// ResetPingTimer is not thread safe
func (px *Proxy) ResetPingTimer() {
	px.pingTimer.Reset(pingTimeout)
}
