package proxy

import (
	"context"

	"github.com/skycoin/teller/src/daemon"
)

func bindHandlers(px *Proxy) {
	px.mux.HandleFunc(daemon.PingMsgType, PingMessageHandler(px))
}

// PingMessageHandler handler for processing the received ping message
func PingMessageHandler(px *Proxy) daemon.Handler {
	return func(ctx context.Context, w daemon.ResponseWriteCloser, msg daemon.Messager) {
		if msg.Type() != daemon.PingMsgType {
			px.log.WithField("msgType", msg.Type()).Error("PingMessageHandler expected PingMsgType")
			return
		}

		px.ResetPingTimer()

		w.Write(&daemon.PongMessage{Value: "PONG"})
		px.log.WithField("msgType", msg.Type()).Debug("Send")
	}
}
