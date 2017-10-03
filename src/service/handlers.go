package service

import (
	"errors"

	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/skycoin/teller/src/daemon"
)

var (
	ErrRequestMessageIsNil = errors.New("request message is nil")
	ErrInternalServError   = errors.New("internal server error")
)

// Gatewayer provides methods to communicate with service.
type Gatewayer interface {
	Logger() *logrus.Logger // FIXME methods should use a context
	ResetPongTimer()        // Reset the session's pong timer
	BindAddress(skyAddr string) (string, error)
	GetDepositStatuses(skyAddr string) ([]daemon.DepositStatus, error)
}

// bindHandlers
func bindHandlers(srv *Service) {
	srv.HandleFunc(daemon.PongMsgType, PongMessageHandler(srv.gateway))
	srv.HandleFunc(daemon.BindRequestMsgType, BindRequestHandler(srv.gateway))
	srv.HandleFunc(daemon.StatusRequestMsgType, StatusRequestHandler(srv.gateway))
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
			gw.Logger().Printf("Expect pong message, but got:%v\n", msg.Type())
			return
		}

		gw.ResetPongTimer()
	}
}

// BindRequestHandler process bind request
func BindRequestHandler(gw Gatewayer) daemon.Handler {
	return func(w daemon.ResponseWriteCloser, msg daemon.Messager) {
		if msg == nil {
			gw.Logger().Println(ErrRequestMessageIsNil)
			return
		}

		gw.ResetPongTimer()

		if msg.Type() != daemon.BindRequestMsgType {
			gw.Logger().Printf("Expect bind request message, but got: %v\n", msg.Type())
			return
		}

		req, ok := msg.(*daemon.BindRequest)
		if !ok {
			gw.Logger().Println("Assert *daemon.BindRequest failed")
			return
		}

		ack := daemon.BindResponse{}
		ack.Id = req.ID()

		btcAddr, err := gw.BindAddress(req.SkyAddress)
		if err != nil {
			gw.Logger().Printf("Bind address failed: %v", err)
			ack.Error = fmt.Sprintf("Bind address failed: %v", err)
		} else {
			ack.BtcAddress = btcAddr
		}

		gw.Logger().Printf("BindRequestHandler req=%+v ack=%+v\n", *req, ack)

		w.Write(&ack)
	}
}

// StatusRequestHandler handler for processing status request
func StatusRequestHandler(gw Gatewayer) daemon.Handler {
	return func(w daemon.ResponseWriteCloser, msg daemon.Messager) {
		if msg == nil {
			gw.Logger().Println(ErrRequestMessageIsNil)
			return
		}

		gw.ResetPongTimer()

		if msg.Type() != daemon.StatusRequestMsgType {
			gw.Logger().Printf("Expect status request message, but got:%v\n", msg.Type())
			return
		}

		req, ok := msg.(*daemon.StatusRequest)
		if !ok {
			gw.Logger().Println("Assert *daemon.StatusRequest failed")
			return
		}

		ack := daemon.StatusResponse{}
		ack.Id = req.ID()

		sts, err := gw.GetDepositStatuses(req.SkyAddress)
		if err != nil {
			errStr := fmt.Sprintf("Get status of %s failed: %v", req.SkyAddress, err)
			gw.Logger().Println(errStr)
			ack.Error = errStr
		} else {
			ack.Statuses = sts
		}

		gw.Logger().Printf("StatusRequestHandler req=%+v ack=%+v\n", *req, ack)

		w.Write(&ack)
	}
}
