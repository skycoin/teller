package proxy

import "github.com/skycoin/teller/src/daemon"

type sessionMgr struct {
	sns chan *daemon.Session
}
