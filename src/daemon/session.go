package daemon

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/skycoin/teller/src/logger"
)

var (
	// ErrWriteChanFull is returned when the write channel is full
	ErrWriteChanFull = errors.New("write channel is full")
	// ErrSessionClosed is returned if the session has closed
	ErrSessionClosed = errors.New("session is closed")
)

const (
	handleTimeout  = 5 * time.Second
	readBufferSize = 1024
)

// ResponseWriteCloser will be used to write data back, also provides Close method
// to close the session if necessary.
type ResponseWriteCloser interface {
	Write(data Messager)
	Close()
}

// Handler callback function when receiving message.
type Handler func(ctx context.Context, w ResponseWriteCloser, data Messager)

// Mux for records the message handlers
type Mux struct {
	handlers map[MsgType]Handler
	log      logrus.FieldLogger
}

// NewMux creates mux
func NewMux(log logrus.FieldLogger) *Mux {
	return &Mux{
		handlers: make(map[MsgType]Handler),
		log: log.WithFields(logrus.Fields{
			"obj":    "Mux",
			"prefix": "daemon",
		}),
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
func (m *Mux) Handle(ctx context.Context, w ResponseWriteCloser, msg Messager) {
	if hd, ok := m.handlers[msg.Type()]; ok {
		m.log.Debugln("Handling msg type", msg.Type())
		hd(ctx, w, msg)
		return
	}
	m.log.Debugln("No handler found for msg type", msg.Type())
}

// Session represents a connection Session, when this Session is done, the connection will be close.
// Session will read message from transport and dispatch the message to the mux.
type Session struct {
	mux       *Mux
	ts        *transport
	wc        chan Messager // write channel
	quit      chan struct{}
	log       logrus.FieldLogger
	subs      map[int]func(Messager) // subscribers
	idGenC    chan int               // subscribe id generator channel
	reqC      chan func()
	wcBufSize int
}

// NewSession creates a new session
func NewSession(log logrus.FieldLogger, conn net.Conn, auth *Auth, mux *Mux, solicited bool, ops ...Option) (*Session, error) {
	if auth == nil {
		return nil, errors.New("auth is nil")
	}

	// create transport
	ts, err := newTransport(conn, auth, solicited)
	if err != nil {
		return nil, err
	}

	s := &Session{
		mux:       mux,
		ts:        ts,
		wcBufSize: 100, // default value, can be changed by Option
		quit:      make(chan struct{}),
		log: log.WithFields(logrus.Fields{
			"prefix": "daemon",
			"obj":    "Session",
		}),
		subs:   make(map[int]func(Messager)),
		idGenC: make(chan int),
		reqC:   make(chan func()),
	}

	for _, op := range ops {
		op(s)
	}

	s.wc = make(chan Messager, s.wcBufSize)

	return s, nil
}

// Run starts the session process loop
func (sn *Session) Run() error {
	msgChan := make(chan Messager, readBufferSize)
	errC := make(chan error, 1)

	go func() {
		// read message loop
		for {
			msg, err := sn.ts.Read()
			if err != nil {
				// check if read fail was caused by session close
				select {
				case <-sn.quit:
					return
				default:
				}

				errC <- err
				return
			}

			select {
			case msgChan <- msg:
			case <-time.After(5 * time.Second):
				sn.log.Debug("Put message timeout")
				return
			}
		}
	}()

	idC := make(chan chan struct{}, 1)

	// start the session subscribe id generator
	go func() {
		i := 1
		for {
			select {
			case c := <-idC:
				c <- struct{}{}
				return
			case sn.idGenC <- i:
				i++
				i = i % 2048 // limit the max id number
			}
		}
	}()

	defer func() {
		c := make(chan struct{}, 1)
		idC <- c
		<-c
	}()

	ctx := logger.WithContext(context.Background(), sn.log)

	for {
		select {
		case <-sn.quit:
			return nil
		case err := <-errC:
			sn.log.WithError(err).Debug()
			return err
		case req := <-sn.reqC:
			req()
		case msg := <-msgChan:
			sn.log.WithField("msgType", msg.Type()).Debug("Recv msg")
			if sn.mux != nil {
				sn.mux.Handle(ctx, sn, msg)
			}

			// find the message subscriber and push data
			if push, ok := sn.subs[msg.ID()]; ok {
				go push(msg)
			}

		case msg := <-sn.wc:
			if err := sn.ts.Write(msg); err != nil {
				sn.log.WithError(err).Debug()
				return err
			}
		}
	}
}

// Close cancel the session
func (sn *Session) Close() {
	close(sn.quit)

	if err := sn.ts.Close(); err != nil {
		sn.log.WithError(err).Error()
	}
}

// Write push the message into write channel.
func (sn *Session) Write(msg Messager) {
	select {
	case <-sn.quit:
		return
	case sn.wc <- msg:
	case <-time.After(writeTimeout):
		sn.log.WithError(ErrWriteChanFull).Error()
		sn.Close()
	}
}

// WriteWithContext write message
func (sn *Session) WriteWithContext(ctx context.Context, msg Messager) error {
	select {
	case <-sn.quit:
		return ErrSessionClosed
	case sn.wc <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
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
