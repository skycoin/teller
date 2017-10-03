package service

import (
	"testing"

	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/util/logger"
	"github.com/stretchr/testify/require"
)

type dummyGateway struct {
	// resetPongTimer bool
	logger.Logger
	bindErr error
	skyAddr string
	btcAddr string
}

func (dg *dummyGateway) ResetPongTimer() {
	// dg.resetPongTimer = true
}

func (dg *dummyGateway) BindAddress(skyAddr string) (string, error) {
	dg.skyAddr = skyAddr
	return dg.btcAddr, dg.bindErr
}

func (dg *dummyGateway) GetDepositStatuses(skyAddr string) ([]daemon.DepositStatus, error) {
	return nil, nil
}

type ResWC struct {
	ackMsg daemon.Messager
	closed bool
	write  bool
}

func (wc *ResWC) Write(m daemon.Messager) {
	wc.write = true
	wc.ackMsg = m
}

func (wc *ResWC) Close() {
	wc.closed = true
}

func TestBindMessage(t *testing.T) {
	dg := dummyGateway{
		Logger:  logger.NewLogger("", true),
		btcAddr: "14JwrdSxYXPxSi6crLKVwR4k2dbjfVZ3xj",
	}

	hd := BindRequestHandler(&dg)
	w := ResWC{}
	br := daemon.BindRequest{SkyAddress: "a1"}
	br.Id = 1

	hd(&w, &br)
	require.Equal(t, "a1", dg.skyAddr)
	require.Equal(t, 1, w.ackMsg.ID())
	require.Equal(t, daemon.BindResponseMsgType, w.ackMsg.Type())
	require.Equal(t, dg.btcAddr, w.ackMsg.(*daemon.BindResponse).BtcAddress)
}
