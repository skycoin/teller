package proxy

import (
	"github.com/skycoin/teller/src/daemon"
)

func bindHandlers(px *Proxy) {
	px.mux.HandleFunc(daemon.PingMsgType, PingMessageHandler(px))
}

// PingMessageHandler handler for processing the received ping message
func PingMessageHandler(px *Proxy) daemon.Handler {
	return func(w daemon.ResponseWriteCloser, msg daemon.Messager) {
		if msg.Type() != daemon.PingMsgType {
			px.log.Printf("Mux error, dispatch %s message to ping message handler\n", msg.Type())
			return
		}

		px.ResetPingTimer()

		w.Write(&daemon.PongMessage{Value: "PONG"})
		px.log.Debugln("Send pong message")
	}
}
