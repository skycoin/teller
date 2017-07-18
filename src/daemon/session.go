package daemon

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/skycoin/teller/src/logger"
)

var (
	ErrWriteChanFull = errors.New("Write channel is full")
)

// ResponseWriteCloser will be used to write data back, also provides Close method
// to close the session if necessary.
type ResponseWriteCloser interface {
	Write(data Messager)
	Close()
}

// Handler callback function when receiving message.
type Handler func(w ResponseWriteCloser, data Messager)

// Mux for records the message handlers
type Mux struct {
	handlers map[MsgType]Handler
	log      logger.Logger
}

// NewMux creates mux
func NewMux(log logger.Logger) *Mux {
	return &Mux{
		handlers: make(map[MsgType]Handler),
		log:      log,
	}
}

// HandleFunc registers the handle function for the given message type.
func (m *Mux) HandleFunc(tp MsgType, handler Handler) error {
	if _, ok := m.handlers[tp]; ok {
		return MsgAlreadRegisterError{Value: tp.String()}
	}
	m.handlers[tp] = handler
	return nil
}

// Handle process the given message
func (m *Mux) Handle(w ResponseWriteCloser, msg Messager) {
	if hd, ok := m.handlers[msg.Type()]; ok {
		hd(w, msg)
		return
	}
	// m.log.Println(MsgNotRegisterError{Value: msg.Type().String()})
	// w.Close()
}

// Session represents a connection Session, when this Session is done, the connection will be close.
// Session will read message from transport and dispatch the message to the mux.
type Session struct {
	mux    *Mux
	ts     *transport
	wc     chan Messager // write channel
	quit   chan struct{}
	log    logger.Logger
	subs   map[int]func(Messager) // subscribers
	idGenC chan int               // subscribe id generator channel
	reqC   chan func()
}

// NewSession creates a new session
func NewSession(conn net.Conn, auth *Auth, mux *Mux, solicited bool, ops ...Option) (*Session, error) {
	if auth == nil {
		return nil, errors.New("auth is nil")
	}

	// create transport
	ts, err := newTransport(conn, auth, solicited)
	if err != nil {
		return nil, err
	}

	s := &Session{
		mux:    mux,
		ts:     ts,
		wc:     make(chan Messager, 1),
		quit:   make(chan struct{}),
		log:    logger.NewLogger("", false),
		subs:   make(map[int]func(Messager)),
		idGenC: make(chan int),
		reqC:   make(chan func()),
	}

	for _, op := range ops {
		op(s)
	}

	return s, nil
}

// Run starts the session process loop
func (sn *Session) Run(cxt context.Context) error {
	// defer close the q to notify that this session is done.
	defer close(sn.quit)
	msgChan := make(chan Messager)
	errC := make(chan error, 1)
	go func() {
		// read message loop
		defer sn.log.Debugln("Exit read message loop")
		for {
			msg, err := sn.ts.Read()
			if err != nil {
				select {
				case errC <- err:
				case <-time.After(5 * time.Second):
				}
				return
			}
			select {
			case msgChan <- msg:
			case <-time.After(5 * time.Second):
				sn.log.Debugln("Put message timeout")
				return
			}
		}
	}()

	go func() {
		i := 1
		for {
			select {
			case <-cxt.Done():
				return
			case sn.idGenC <- i:
				i++
				i = i % 2048 // limit the max id number
			}
		}
	}()
	for {
		select {
		case <-cxt.Done():
			return nil
		case err := <-errC:
			sn.log.Debugln(err)
			return err
		case msg := <-msgChan:
			sn.log.Debugln("Recv msg:", msg.Type())
			if sn.mux != nil {
				sn.mux.Handle(sn, msg)
			}

			// find the message subscriber and push data
			if push, ok := sn.subs[msg.ID()]; ok {
				go push(msg)
			}

		case req := <-sn.reqC:
			req()
		case msg := <-sn.wc:
			if err := sn.ts.Write(msg); err != nil {
				sn.log.Debugln(err)
				return err
			}
		}
	}
}

// Close cancel the session
func (sn *Session) Close() {
	if err := sn.ts.Close(); err != nil {
		sn.log.Println(err)
	}
	<-sn.quit
}

// Write push the message into write channel.
func (sn *Session) Write(msg Messager) {
	select {
	case sn.wc <- msg:
	case <-time.After(5 * time.Second):
		sn.log.Println(ErrWriteChanFull)
		sn.Close()
	}
}

// Sub the session data stream, will return the subID for later unsubscribe
func (sn *Session) Sub(fn func(Messager)) int {
	id := <-sn.idGenC
	sn.strand(func() {
		sn.subs[id] = fn
	})
	return id
}

// Unsub unsubscribe the data stream
func (sn *Session) Unsub(id int) {
	sn.strand(func() {
		delete(sn.subs, id)
	})
}

func (sn *Session) strand(f func()) {
	q := make(chan struct{})
	sn.reqC <- func() {
		defer close(q)
		f()
	}
	<-q
}
