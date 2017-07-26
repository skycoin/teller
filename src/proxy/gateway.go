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
	logger.Logger
	BindAddress(context.Context, *daemon.BindRequest) (*daemon.BindResponse, error)
	// AddMonitor(context.Context, *daemon.MonitorMessage) (*daemon.MonitorAckMessage, error)
	// GetExchangeLogs(context.Context, *daemon.GetExchangeLogsMessage) (*daemon.GetExchangeLogsAckMessage, error)
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

// func (gw *gateway) AddMonitor(cxt context.Context, msg *daemon.MonitorMessage) (*daemon.MonitorAckMessage, error) {
// 	var ack daemon.MonitorAckMessage
// 	if err := gw.sendMessage(cxt, msg, &ack); err != nil {
// 		return nil, err
// 	}
// 	return &ack, nil
// }

// func (gw *gateway) GetExchangeLogs(cxt context.Context, msg *daemon.GetExchangeLogsMessage) (*daemon.GetExchangeLogsAckMessage, error) {
// 	var ack daemon.GetExchangeLogsAckMessage
// 	if err := gw.sendMessage(cxt, msg, &ack); err != nil {
// 		return nil, err
// 	}
// 	return &ack, nil
// }

func (gw *gateway) sendMessage(cxt context.Context, msg daemon.Messager, ackMsg daemon.Messager) (err error) {
	// the ackMsg must be
	if reflect.TypeOf(ackMsg).Kind() != reflect.Ptr {
		return errors.New("Ack message type must be setable")
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

		go func() {
			if err = gw.p.write(msg); err != nil {
				return
			}
		}()

		select {
		case <-cxt.Done():
			err = cxt.Err()
			return
		case ack := <-msgC:
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
