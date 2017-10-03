package daemon

import "github.com/skycoin/teller/src/util/logger"

// Option session's optional argument type
type Option func(*Session)

// Logger sets session's log
func Logger(log logger.Logger) Option {
	return func(s *Session) {
		s.log = log
	}
}

func WriteBufferSize(bufsize int) Option {
	return func(s *Session) {
		s.wcBufSize = bufsize
	}
}
