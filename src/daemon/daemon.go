package daemon

// Server a tcp server, all connection sessions are encrypted.
// type Server struct {
// 	Port    int
// 	Address string

// 	listener   net.Listener  // tcp socket listener
// 	connChan   chan net.Conn // connection channel
// 	workerChan chan *worker  // worker channel
// }

// Cancel callback function for shutting down the server
// type Cancel func()

// NewServer creates a server instance
// func NewServer(addr string, port int, auth *Auth, mux *Mux, ops ...Option) (*Server, Cancel) {
// 	s := &Server{
// 		Address:    addr,
// 		Port:       port,
// 		connChan:   make(chan net.Conn, 1),
// 		workerChan: make(chan *worker, 1),
// 	}

// 	for _, op := range ops {
// 		op(s)
// 	}

// 	// create withCancel context
// 	cxt, cancel := context.WithCancel(context.Background())

// 	// create the only one worker
// 	wrk := newWorker(cxt, s.workerChan, auth, mux, false)

// 	// push the worker into pool
// 	s.workerChan <- wrk

// 	s.run(cxt)

// 	return s, func() {
// 		cancel()
// 		if s.listener != nil {
// 			s.listener.Close()
// 			s.listener = nil
// 		}
// 	}
// }

// // Run start the server, none-blocking, only one connection is allowed.
// func (s *Server) run(cxt context.Context) {
// 	var err error
// 	s.listener, err = net.Listen("tcp", fmt.Sprintf("%v:%v", s.Address, s.Port))
// 	if err != nil {
// 		panic(err)
// 	}

// 	// start the connection handler
// 	go s.handleConnection(cxt)

// 	go func() {
// 		for {
// 			conn, err := s.listener.Accept()
// 			if err != nil {
// 				select {
// 				case <-cxt.Done():
// 					glog.Info("server exit")
// 					return
// 				default:
// 					// without the default case the select will block
// 				}
// 				continue
// 			}

// 			s.connChan <- conn
// 		}
// 	}()
// }

// // handles the connection from pool, only one connection is allowed.
// func (s *Server) handleConnection(cxt context.Context) {
// 	for {
// 		select {
// 		case <-cxt.Done():
// 			return
// 		case conn := <-s.connChan:
// 			// have worker to process the connection
// 			select {
// 			case <-time.After(3 * time.Second):
// 				// no worker is available, cause we only have one worker, and if there's one that is working
// 				// as designed, no more connection is allowed to connect to the server.
// 				conn.Close()
// 			case wrk := <-s.workerChan:
// 				wrk.c <- conn
// 			}
// 		}
// 	}
// }
