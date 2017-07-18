package daemon

import "github.com/skycoin/teller/src/logger"

// Option session's optional argument type
type Option func(*Session)

// Logger sets session's log
func Logger(log logger.Logger) Option {
	return func(s *Session) {
		s.log = log
	}
}
