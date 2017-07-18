package service

import (
	"errors"
	"reflect"

	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/logger"
)

var (
	ErrRequestMessageIsNil = errors.New("Request message is nil")
	ErrInternalServError   = errors.New("Internal server error")
)

// Gatewayer provides methods to communicate with service.
type Gatewayer interface {
	Logger() logger.Logger
	ResetPongTimer()                                              // Reset the session's pong timer
	AddMonitor(*daemon.MonitorMessage) error                      // add new coin address to monitor
	GetExchangeLogs(start, end int) ([]daemon.ExchangeLog, error) // get logs whose id are in the given range
	GetExchangeLogsLen() int                                      // return the exchange logs length
}

// bindHandlers
func bindHandlers(srv *Service) {
	srv.HandleFunc(daemon.PongMsgType, PongMessageHandler(srv.gateway))
	srv.HandleFunc(daemon.MonitorMsgType, MonitorMessageHandler(srv.gateway))
	srv.HandleFunc(daemon.GetExchgLogsMsgType, GetExchangeLogsHandler(srv.gateway))
}

// PongMessageHandler handler for processing the received pong message
func PongMessageHandler(gw Gatewayer) daemon.Handler {
	return func(w daemon.ResponseWriteCloser, msg daemon.Messager) {
		if msg == nil {
			gw.Logger().Println(ErrRequestMessageIsNil)
			return
		}
		// reset the service's pong timer
		pm := daemon.PongMessage{}
		if msg.Type() != pm.Type() {
			gw.Logger().Printf("Expect pong message, while got:%v\n", msg.Type())
			return
		}

		gw.ResetPongTimer()
	}
}

// MonitorMessageHandler handler for processing the monitor message
func MonitorMessageHandler(gw Gatewayer) daemon.Handler {
	return func(w daemon.ResponseWriteCloser, msg daemon.Messager) {
		if msg == nil {
			gw.Logger().Println(ErrRequestMessageIsNil)
			return
		}

		mm := &daemon.MonitorMessage{}
		if msg.Type() != mm.Type() {
			gw.Logger().Printf("Expect monitor message, but got:%v\n", msg.Type())
			return
		}

		mm, ok := msg.(*daemon.MonitorMessage)
		if !ok {
			gw.Logger().Println("Assert *daemon.MonitorMessage failed")
			return
		}

		if err := gw.AddMonitor(mm); err != nil {
			gw.Logger().Debugln("Add monitor failed,", err)
			ack := daemon.MonitorAckMessage{
				Base: daemon.Base{
					Id: mm.ID(),
				},
				Result: daemon.Result{
					Success: false,
					Err:     err.Error(),
				},
			}

			w.Write(&ack)
			return
		}
		// send ack back
		ack := daemon.MonitorAckMessage{
			Base: daemon.Base{
				Id: mm.ID(),
			},
			Result: daemon.Result{
				Success: true,
			},
		}
		w.Write(&ack)
	}
}

// GetExchangeLogsHandler handler for processing GetExchangeLogsMessage
func GetExchangeLogsHandler(gw Gatewayer) daemon.Handler {
	return func(w daemon.ResponseWriteCloser, msg daemon.Messager) {
		if msg == nil {
			gw.Logger().Println(ErrRequestMessageIsNil)
			return
		}

		if msg.Type() != daemon.GetExchgLogsMsgType {
			gw.Logger().Printf("Expect %s message, but got:%v\n", daemon.GetExchgLogsMsgType, msg.Type())
			return
		}

		req, ok := msg.(*daemon.GetExchangeLogsMessage)
		if !ok {
			gw.Logger().Printf("Failed assert %v to *daemon.GetExchangeLogsMessage failed", reflect.TypeOf(msg))
			return
		}

		logs, err := gw.GetExchangeLogs(req.StartID, req.EndID)
		if err != nil {
			gw.Logger().Printf("Get exchange logs failed, err: %v", err)
			ack := daemon.GetExchangeLogsAckMessage{
				Base: req.Base,
				Result: daemon.Result{
					Success: false,
					Err:     err.Error(),
				},
			}
			w.Write(&ack)
			return
		}

		// make ack message
		ack := daemon.GetExchangeLogsAckMessage{
			Base: req.Base,
			Result: daemon.Result{
				Success: true,
			},
			MaxLogID: gw.GetExchangeLogsLen(),
			Logs:     logs,
		}
		w.Write(&ack)
	}
}
