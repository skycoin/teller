package service

import (
	"errors"

	"fmt"

	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/logger"
)

var (
	ErrRequestMessageIsNil = errors.New("Request message is nil")
	ErrInternalServError   = errors.New("Internal server error")
)

// Gatewayer provides methods to communicate with service.
type Gatewayer interface {
	logger.Logger
	ResetPongTimer() // Reset the session's pong timer
	BindAddress(skyAddr string) (string, error)
	// AddMonitor(*daemon.MonitorMessage) error                      // add new coin address to monitor
	// GetExchangeLogs(start, end int) ([]daemon.ExchangeLog, error) // get logs whose id are in the given range
	// GetExchangeLogsLen() int                                      // return the exchange logs length
}

// bindHandlers
func bindHandlers(srv *Service) {
	srv.HandleFunc(daemon.PongMsgType, PongMessageHandler(srv.gateway))
	srv.HandleFunc(daemon.BindRequestMsgType, BindRequestHandler(srv.gateway))
}

// PongMessageHandler handler for processing the received pong message
func PongMessageHandler(gw Gatewayer) daemon.Handler {
	return func(w daemon.ResponseWriteCloser, msg daemon.Messager) {
		if msg == nil {
			gw.Println(ErrRequestMessageIsNil)
			return
		}
		// reset the service's pong timer
		pm := daemon.PongMessage{}
		if msg.Type() != pm.Type() {
			gw.Printf("Expect pong message, but got:%v\n", msg.Type())
			return
		}

		gw.ResetPongTimer()
	}
}

// BindRequestHandler process bind request
func BindRequestHandler(gw Gatewayer) daemon.Handler {
	return func(w daemon.ResponseWriteCloser, msg daemon.Messager) {
		if msg == nil {
			gw.Println(ErrRequestMessageIsNil)
			return
		}

		if msg.Type() != daemon.BindRequestMsgType {
			gw.Printf("Expect bind request message, but got: %v\n", msg.Type())
			return
		}

		req, ok := msg.(*daemon.BindRequest)
		if !ok {
			gw.Println("Assert *daemon.BindRequest failed")
			return
		}

		ack := daemon.BindResponse{}
		ack.Id = req.ID()

		btcAddr, err := gw.BindAddress(req.SkyAddress)
		if err != nil {
			gw.Printf("Bind address failed: %v", err)
			ack.Err = fmt.Sprintf("Bind address failed: %v", err)
		} else {
			ack.BtcAddress = btcAddr
		}

		w.Write(&ack)
	}
}
