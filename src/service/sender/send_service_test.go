package sender

import (
	"errors"
	"sync"
	"testing"

	"time"

	"github.com/skycoin/teller/src/logger"
	"github.com/skycoin/teller/src/service/cli"
	"github.com/stretchr/testify/require"
)

type dummySkycli struct {
	sync.Mutex
	sendTxid    string
	sendErr     error
	txConfirmed bool
	getTxErr    error
}

func newDummySkycli() *dummySkycli {
	return &dummySkycli{}
}

func (ds *dummySkycli) Send(addr string, coins int64) (string, error) {
	return ds.sendTxid, ds.sendErr
}

func (ds *dummySkycli) GetTransaction(txid string) (*cli.TxJSON, error) {
	ds.Lock()
	defer ds.Unlock()
	txjson := cli.TxJSON{}
	txjson.Transaction.Status.Confirmed = ds.txConfirmed
	return &txjson, ds.getTxErr
}

func (ds *dummySkycli) changeConfirmStatus(v bool) {
	ds.Lock()
	defer ds.Unlock()
	ds.txConfirmed = v
}

func (ds *dummySkycli) changeSendErr(err error) {
	ds.Lock()
	defer ds.Unlock()
	ds.sendErr = err
}

func (ds *dummySkycli) changeGetTxErr(err error) {
	ds.Lock()
	defer ds.Unlock()
	ds.getTxErr = err
}

func TestSendService(t *testing.T) {
	log := logger.NewLogger("", true)
	dsc := newDummySkycli()
	dsc.sendTxid = "1111"
	s := NewService(Config{}, log, dsc)
	go func() {
		s.Run()
	}()

	time.AfterFunc(500*time.Millisecond, func() {
		dsc.changeConfirmStatus(true)
	})

	addr := "KNtZkX2mw1UFuemv6FmEQxxhWCTWTm2Thk"

	sdr := NewSender(s)
	txid, err := sdr.Send(addr, 10, nil)
	require.Nil(t, err)
	require.Equal(t, "1111", txid)

	// test send coin failed
	dsc.sendErr = errors.New("connect to node failed")
	time.AfterFunc(5*time.Second, func() {
		dsc.changeSendErr(nil)
	})

	txid, err = sdr.Send(addr, 20, nil)
	require.Nil(t, err)
	require.Equal(t, "1111", txid)

	// test get transaction failed
	dsc.getTxErr = errors.New("get transaction failed")
	time.AfterFunc(5*time.Second, func() {
		dsc.changeGetTxErr(nil)
	})

	txid, err = sdr.Send(addr, 20, nil)
	require.Nil(t, err)
	require.Equal(t, "1111", txid)

	// test send timeout
	dsc.changeConfirmStatus(false)
	time.AfterFunc(time.Second, func() {
		_, err = sdr.Send(addr, 20, &SendOption{
			Timeout: time.Second * 4,
		})
		require.Equal(t, ErrSendBufferFull, err)
		s.Shutdown()
	})

	_, err = sdr.Send(addr, 10, nil)
	require.Equal(t, ErrServiceClosed, err)
}
