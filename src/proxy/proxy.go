// Package proxy is the service run in the public server, and provides
// http apis for web server. The proxy use tcp socket to communicate with
// client, and all data are encrypted by ECDH and chacha20.
package proxy

import (
	"context"
	"errors"
	"net"

	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/logger"

	"time"

	"sync"

	"io"
)

// Proxy represents the ico proxy server
type Proxy struct {
	srvAddr     string // listen address, eg: 0.0.0.0:12345
	httpSrvAddr string
	ln          net.Listener
	cxt         context.Context
	log         logger.Logger
	sn          *daemon.Session
	connC       chan net.Conn
	wg          *sync.WaitGroup
	auth        *daemon.Auth
	mux         *daemon.Mux
	reqC        chan func()

	httpServ *httpServ
}

// Cancel callback function for stopping the proxy
type Cancel func()

// New creates proxy instance
func New(srvAddr, httpSrvAddr string, auth *daemon.Auth, ops ...Option) (*Proxy, Cancel) {
	if auth == nil {
		panic("Auth is nil")
	}

	cxt, cancel := context.WithCancel(context.Background())
	px := &Proxy{
		srvAddr:     srvAddr,
		httpSrvAddr: httpSrvAddr,
		cxt:         cxt,
		log:         logger.NewLogger("", false), // default logger does not turn on debug mode, can use Logger option to set it.
		connC:       make(chan net.Conn),
		auth:        auth,
		reqC:        make(chan func()),
		wg:          &sync.WaitGroup{},
	}

	for _, op := range ops {
		op(px)
	}

	px.mux = daemon.NewMux(px.log)

	bindHandlers(px)

	px.httpServ = newHTTPServ(httpSrvAddr, px.log, &gateway{p: px, Logger: px.log})

	return px, func() {
		cancel()
		px.wg.Wait()
	}
}

// Run start the proxy
func (px *Proxy) Run() {
	var err error
	px.ln, err = net.Listen("tcp", px.srvAddr)
	if err != nil {
		panic(err)
	}

	px.log.Println("Proxy start, serve on", px.srvAddr)

	// start connection handler process
	px.wg.Add(1)
	go func() {
		defer px.wg.Done()
		px.handleConnection()
	}()

	px.wg.Add(1)
	go func() {
		defer px.wg.Done()
		for {
			conn, err := px.ln.Accept()
			if err != nil {
				select {
				case <-px.cxt.Done():
					return
				default:
					px.log.Println("Accept error:", err)
					continue
				}
			}

			select {
			case <-time.After(1 * time.Second):
				px.log.Debugf("Close connection:%s, only one connection is allowed\n", conn.RemoteAddr())
				conn.Close()
			case px.connC <- conn:
			}
		}
	}()

	px.wg.Add(1)
	go func() {
		defer func() {
			px.wg.Done()
			px.log.Debugln("Http service stop")
		}()

		px.httpServ.Run(px.cxt)
	}()

	px.wg.Add(1)
	go func() {
		defer func() {
			px.wg.Done()
			px.log.Println("Proxy stop")
		}()
		for {
			select {
			case <-px.cxt.Done():
				px.close()
				return
			case req := <-px.reqC:
				req()
			}
		}
	}()
}

func (px *Proxy) close() {
	if px.ln != nil {
		px.ln.Close()
		px.ln = nil
	}
}

func (px *Proxy) handleConnection() {
	execFuncC := make(chan func(conn net.Conn), 1)
	execFuncC <- px.newSession
	for {
		select {
		case <-px.cxt.Done():
			return
		case conn := <-px.connC:
			select {
			case <-time.After(2 * time.Second):
				px.log.Debugf("Close connection %s, only one connection is allowed", conn.RemoteAddr())
				conn.Close()
				return
			case exec := <-execFuncC:
				exec(conn)
				conn.Close()
				execFuncC <- exec
			}
		}
	}
}

func (px *Proxy) newSession(conn net.Conn) {
	px.log.Debugln("New session")
	defer px.log.Debugln("Session closed")
	var err error
	px.sn, err = daemon.NewSession(conn, px.auth, px.mux, false, daemon.Logger(px.log))
	if err != nil {
		px.log.Debug(err)
		return
	}

	if err := px.sn.Run(); err != nil {
		if err != io.EOF {
			px.log.Debug(err)
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
		return errors.New("Write failed, session is nil")
	}

	px.sn.Write(m)
	return nil
}

type closeStream func()

// openStream
func (px *Proxy) openStream(f func(daemon.Messager)) (int, closeStream, error) {
	if px.sn == nil {
		return 0, func() {}, errors.New("Session is nil")
	}

	id := px.sn.Sub(f)
	px.log.Debugln("Open stream:", id)
	return id, func() {
		px.sn.Unsub(id)
		px.log.Debugln("Close stream:", id)
	}, nil
}
