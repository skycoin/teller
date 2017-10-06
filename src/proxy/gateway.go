package proxy

import (
	"context"
	"errors"
	"reflect"

	"fmt"

	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/logger"
)

type gatewayer interface {
	BindAddress(context.Context, *daemon.BindRequest) (*daemon.BindResponse, error)
	GetDepositStatuses(context.Context, *daemon.StatusRequest) (*daemon.StatusResponse, error)
}

type gateway struct {
	p *Proxy
}

func (gw *gateway) BindAddress(ctx context.Context, req *daemon.BindRequest) (*daemon.BindResponse, error) {
	var rsp daemon.BindResponse
	if err := gw.sendMessage(ctx, req, &rsp); err != nil {
		return nil, err
	}

	return &rsp, nil
}

func (gw *gateway) GetDepositStatuses(ctx context.Context, req *daemon.StatusRequest) (*daemon.StatusResponse, error) {
	var rsp daemon.StatusResponse
	if err := gw.sendMessage(ctx, req, &rsp); err != nil {
		return nil, err
	}

	return &rsp, nil
}

func (gw *gateway) sendMessage(ctx context.Context, msg daemon.Messager, ackMsg daemon.Messager) (err error) {
	// the ackMsg must be
	if reflect.TypeOf(ackMsg).Kind() != reflect.Ptr {
		return errors.New("ack message type must be setable")
	}

	log := logger.FromContext(ctx)

	gw.p.strand(func() {
		msgC := make(chan daemon.Messager, 1)
		// open the data stream
		id, closeStream, er := gw.p.openStream(func(m daemon.Messager) {
			log.WithField("msgType", m.Type()).Debug("Recv message")
			msgC <- m
		})
		if er != nil {
			err = er
			return
		}
		defer closeStream()

		// send  message
		msg.SetID(id)

		if err = gw.p.writeWithContext(ctx, msg); err != nil {
			return
		}
		select {
		case <-ctx.Done():
			err = ctx.Err()
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
