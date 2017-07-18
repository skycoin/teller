package proxy

import (
	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/logger"
)

func bindHandlers(px *Proxy) {
	px.mux.HandleFunc(daemon.PingMsgType, PingMessageHandler(px.log))
}

// PingMessageHandler handler for processing the received ping message
func PingMessageHandler(log logger.Logger) daemon.Handler {
	return func(w daemon.ResponseWriteCloser, msg daemon.Messager) {
		if msg.Type() != daemon.PingMsgType {
			log.Printf("Mux error, dispatch %s message to ping message handler\n", msg.Type())
			return
		}

		w.Write(&daemon.PongMessage{Value: "PONG"})
		log.Debugln("Send pong message")
	}
}
