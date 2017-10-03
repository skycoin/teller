package service

import (
	"context"
	"errors"

	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/logger"
)

var (
	// ErrRequestMessageIsNil is returned when the request message is nil
	ErrRequestMessageIsNil = errors.New("request message is nil")
	// ErrInternalServError is returned when an internal error occurs
	ErrInternalServError = errors.New("internal server error")
)

// Gatewayer provides methods to communicate with service.
type Gatewayer interface {
	ResetPongTimer() // Reset the session's pong timer
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
	return func(ctx context.Context, w daemon.ResponseWriteCloser, msg daemon.Messager) {
		log := logger.FromContext(ctx)

		if msg == nil {
			log.Println(ErrRequestMessageIsNil)
			return
		}
		// reset the service's pong timer
		pm := daemon.PongMessage{}
		if msg.Type() != pm.Type() {
			log.Printf("Expect pong message, but got:%v\n", msg.Type())
			return
		}

		gw.ResetPongTimer()
	}
}

// BindRequestHandler process bind request
func BindRequestHandler(gw Gatewayer) daemon.Handler {
	return func(ctx context.Context, w daemon.ResponseWriteCloser, msg daemon.Messager) {
		log := logger.FromContext(ctx)

		if msg == nil {
			log.Println(ErrRequestMessageIsNil)
			return
		}

		gw.ResetPongTimer()

		if msg.Type() != daemon.BindRequestMsgType {
			log.Printf("Expect bind request message, but got: %v\n", msg.Type())
			return
		}

		req, ok := msg.(*daemon.BindRequest)
		if !ok {
			log.Println("Assert *daemon.BindRequest failed")
			return
		}

		ack := daemon.BindResponse{}
		ack.Id = req.ID()

		btcAddr, err := gw.BindAddress(req.SkyAddress)
		if err != nil {
			log.Printf("Bind address failed: %v", err)
			ack.Error = fmt.Sprintf("Bind address failed: %v", err)
		} else {
			ack.BtcAddress = btcAddr
		}

		log.WithFields(logrus.Fields{
			"req": req,
			"ack": ack,
		}).Info("BindRequestHandler")

		w.Write(&ack)
	}
}

// StatusRequestHandler handler for processing status request
func StatusRequestHandler(gw Gatewayer) daemon.Handler {
	return func(ctx context.Context, w daemon.ResponseWriteCloser, msg daemon.Messager) {
		log := logger.FromContext(ctx)

		if msg == nil {
			log.Println(ErrRequestMessageIsNil)
			return
		}

		gw.ResetPongTimer()

		if msg.Type() != daemon.StatusRequestMsgType {
			log.Printf("Expect status request message, but got:%v\n", msg.Type())
			return
		}

		req, ok := msg.(*daemon.StatusRequest)
		if !ok {
			log.Println("Assert *daemon.StatusRequest failed")
			return
		}

		ack := daemon.StatusResponse{}
		ack.Id = req.ID()

		sts, err := gw.GetDepositStatuses(req.SkyAddress)
		if err != nil {
			errStr := fmt.Sprintf("Get status of %s failed: %v", req.SkyAddress, err)
			log.Println(errStr)
			ack.Error = errStr
		} else {
			ack.Statuses = sts
		}

		log.WithFields(logrus.Fields{
			"req": req,
			"ack": ack,
		}).Info("StatusRequestHandler")

		w.Write(&ack)
	}
}
