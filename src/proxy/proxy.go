// Package proxy is the service run in the public server, and provides
// http apis for web server. The proxy use tcp socket to communicate with
// client, and all data are encrypted by ECDH and chacha20.
package proxy

import (
	"errors"
	"net"
	"sync"

	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/logger"

	"time"

	"io"
)

// Proxy represents the ico proxy server
type Proxy struct {
	logger.Logger
	srvAddr     string // listen address, eg: 0.0.0.0:12345
	httpSrvAddr string
	ln          net.Listener
	quit        chan struct{}
	sn          *daemon.Session
	connC       chan net.Conn
	// wg    *sync.WaitGroup
	auth *daemon.Auth
	mux  *daemon.Mux
	reqC chan func()

	httpServ *httpServ
}

type ProxyConfig struct {
	SrvAddr       string
	HttpSrvAddr   string
	HtmlInterface bool
	HtmlStaticDir string
	StartAt       time.Time
	// If Tls is true, either TlsHost must be set, or both TlsCert and TlsKey must be set
	// If TlsHost is set then TlsCert and TlsKey must not be set, and vice versa
	Tls         bool
	AutoTlsHost string
	TlsCert     string
	TlsKey      string
}

// New creates proxy instance
func New(cfg ProxyConfig, auth *daemon.Auth, ops ...Option) *Proxy {
	if auth == nil {
		panic("Auth is nil")
	}

	if cfg.Tls && cfg.AutoTlsHost == "" && (cfg.TlsCert == "" || cfg.TlsKey == "") {
		panic("when using -tls, either -auto-tls-host or both -tls-cert and -tls-key must be set")
	}

	if (cfg.TlsCert == "" && cfg.TlsKey != "") || (cfg.TlsCert != "" && cfg.TlsKey == "") {
		panic("-tls-cert and -tls-key must be set or unset together")
	}

	if cfg.AutoTlsHost != "" && (cfg.TlsKey != "" || cfg.TlsCert != "") {
		panic("either use -auto-tls-host or both -tls-key and -tls-cert")
	}

	if !cfg.Tls && (cfg.AutoTlsHost != "" || cfg.TlsKey != "" || cfg.TlsCert != "") {
		panic("-auto-tls-host or -tls-key or -tls-cert is set but -tls is not enabled")
	}

	px := &Proxy{
		// default logger does not turn on debug mode, can use Logger option to set it.
		Logger:      logger.NewLogger("", false),
		srvAddr:     cfg.SrvAddr,
		httpSrvAddr: cfg.HttpSrvAddr,
		connC:       make(chan net.Conn),
		auth:        auth,
		reqC:        make(chan func()),
		quit:        make(chan struct{}),
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
		Addr:          cfg.HttpSrvAddr,
		StaticDir:     cfg.HtmlStaticDir,
		HtmlInterface: cfg.HtmlInterface,
		StartAt:       cfg.StartAt,
		Tls:           cfg.Tls,
		AutoTlsHost:   cfg.AutoTlsHost,
		TlsCert:       cfg.TlsCert,
		TlsKey:        cfg.TlsKey,
		Gateway:       gw,
	}

	return px
}

// Run start the proxy
func (px *Proxy) Run() error {
	var err error
	px.ln, err = net.Listen("tcp", px.srvAddr)
	if err != nil {
		return err
	}

	px.Println("Proxy start, serve on", px.srvAddr)
	defer px.Println("Proxy service closed")

	// start connection handler process
	var wg sync.WaitGroup
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

	wg.Add(1)
	go func() {
		defer wg.Done()

		px.httpServ.Run()
	}()

	wg.Wait()
	return nil
}

// Shutdown close the proxy service
func (px *Proxy) Shutdown() {
	close(px.quit)

	if px.ln != nil {
		px.ln.Close()
		px.ln = nil
	}

	if px.sn != nil {
		px.sn.Close()
	}

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
				conn.Close()
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
	var err error
	px.sn, err = daemon.NewSession(conn, px.auth, px.mux, false, daemon.Logger(px.Logger))
	if err != nil {
		px.Println(err)
		return
	}

	if err := px.sn.Run(); err != nil {
		if err != io.EOF {
			px.Println(err)
		}
		return
	}
}

func (px *Proxy) strand(f func()) {
	q := make(chan struct{})
	px.reqC <- func() {
		defer close(q)
		f()
	}
	<-q
}

func (px *Proxy) write(m daemon.Messager) error {
	if px.sn == nil {
		return errors.New("write failed, session is nil")
	}

	px.sn.Write(m)
	return nil
}

type closeStream func()

// openStream
func (px *Proxy) openStream(f func(daemon.Messager)) (int, closeStream, error) {
	if px.sn == nil {
		return 0, func() {}, errors.New("session is nil")
	}

	id := px.sn.Sub(f)
	px.Debugln("Open stream:", id)
	return id, func() {
		px.sn.Unsub(id)
		px.Debugln("Close stream:", id)
	}, nil
}
