package proxy

import "github.com/skycoin/teller/src/util/logger"

// Option represents the proxy option
type Option func(px *Proxy)

// Logger sets the proxy's logger
func Logger(log logger.Logger) Option {
	return func(px *Proxy) {
		px.Logger = log
	}
}
