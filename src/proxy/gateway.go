package proxy

import (
	"context"
	"errors"
	"reflect"

	"fmt"

	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/util/logger"
)

type gatewayer interface {
	logger.Logger
	BindAddress(context.Context, *daemon.BindRequest) (*daemon.BindResponse, error)
	GetDepositStatuses(context.Context, *daemon.StatusRequest) (*daemon.StatusResponse, error)
}

type gateway struct {
	logger.Logger
	p *Proxy
}

func (gw *gateway) BindAddress(cxt context.Context, req *daemon.BindRequest) (*daemon.BindResponse, error) {
	var rsp daemon.BindResponse
	if err := gw.sendMessage(cxt, req, &rsp); err != nil {
		return nil, err
	}

	return &rsp, nil
}

func (gw *gateway) GetDepositStatuses(cxt context.Context, req *daemon.StatusRequest) (*daemon.StatusResponse, error) {
	var rsp daemon.StatusResponse
	if err := gw.sendMessage(cxt, req, &rsp); err != nil {
		return nil, err
	}

	return &rsp, nil
}

func (gw *gateway) sendMessage(cxt context.Context, msg daemon.Messager, ackMsg daemon.Messager) (err error) {
	// the ackMsg must be
	if reflect.TypeOf(ackMsg).Kind() != reflect.Ptr {
		return errors.New("ack message type must be setable")
	}

	gw.p.strand(func() {
		msgC := make(chan daemon.Messager, 1)
		// open the data stream
		id, closeStream, er := gw.p.openStream(func(m daemon.Messager) {
			gw.Debugf("Recv %s message", m.Type())
			msgC <- m
		})
		if er != nil {
			err = er
			return
		}
		defer closeStream()

		// send  message
		msg.SetID(id)

		if err = gw.p.writeWithContext(cxt, msg); err != nil {
			return
		}
		select {
		case <-cxt.Done():
			err = cxt.Err()
			return
		case ack := <-msgC:
			gw.p.ResetPingTimer()
			ackValue := reflect.ValueOf(ack)
			ackMsgValue := reflect.ValueOf(ackMsg)
			if ackValue.Type() != ackMsgValue.Type() {
				err = fmt.Errorf("Can't assign value of type:%v to %v", ackValue.Type(), ackMsgValue.Type())
				return
			}

			reflect.Indirect(reflect.ValueOf(ackMsg)).Set(ackValue.Elem())
			return
		}
	})
	return
}
