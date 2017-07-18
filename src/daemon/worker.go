package daemon

import (
	"context"
	"net"

	"github.com/golang/glog"
)

// worker has one conn channel to recve the new coming connection,
// when worker's process is done, the woker will put back to the worker pool.
// only when the worker is in the pool, it's conn channel has the chance to
// get new connection, this can be used to limit the concourrent inbound connections.
type worker struct {
	c  chan net.Conn
	wc chan *worker
}

func newWorker(cxt context.Context, wc chan *worker, auth *Auth, mux *Mux, solicited bool) *worker {
	w := &worker{
		c:  make(chan net.Conn, 1),
		wc: wc,
	}

	go w.run(cxt, auth, mux, solicited)
	return w
}

func (w *worker) run(cxt context.Context, auth *Auth, mux *Mux, solicited bool) {
	for {
		select {
		case <-cxt.Done():
			return
		case conn := <-w.c:
			if err := w.process(cxt, conn, auth, mux, solicited); err != nil {
				glog.Info("11", err)
				select {
				case w.wc <- w:
				default:
				}
			}
		}
	}
}

func (w *worker) process(cxt context.Context, conn net.Conn, auth *Auth, mux *Mux, solicited bool) error {
	s, err := NewSession(conn, auth, mux, solicited)
	if err != nil {
		return err
	}

	return s.Run(cxt)
}
