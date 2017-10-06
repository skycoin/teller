package daemon

// import (
// 	"context"
// 	"net"

// 	"github.com/skycoin/teller/src/logger"
// )

// // worker has one conn channel to recve the new coming connection,
// // when worker's process is done, the woker will put back to the worker pool.
// // only when the worker is in the pool, it's conn channel has the chance to
// // get new connection, this can be used to limit the concourrent inbound connections.
// type worker struct {
// 	c  chan net.Conn
// 	wc chan *worker
// }

// func newWorker(ctx context.Context, wc chan *worker, auth *Auth, mux *Mux, solicited bool) *worker {
// 	w := &worker{
// 		c:  make(chan net.Conn, 1),
// 		wc: wc,
// 	}

// 	go w.run(ctx, auth, mux, solicited)
// 	return w
// }

// func (w *worker) run(ctx context.Context, auth *Auth, mux *Mux, solicited bool) {
// 	for {
// 		select {
// 		case <-ctx.Done():
// 			return
// 		case conn := <-w.c:
// 			if err := w.process(conn, auth, mux, solicited); err != nil {
// 				// put this worker back to channel
// 				w.wc <- w
// 			}
// 		}
// 	}
// }

// func (w *worker) process(ctx context.Context, conn net.Conn, auth *Auth, mux *Mux, solicited bool) error {
// 	log := logger.FromContext(ctx)

// 	s, err := NewSession(log, conn, auth, mux, solicited)
// 	if err != nil {
// 		return err
// 	}

// 	return s.Run()
// }
