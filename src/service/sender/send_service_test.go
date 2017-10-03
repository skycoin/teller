package sender

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/skycoin/skycoin/src/api/webrpc"
	"github.com/skycoin/skycoin/src/visor"
	"github.com/skycoin/teller/src/service/testutil"
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

func (ds *dummySkycli) Send(addr string, coins uint64) (string, error) {
	return ds.sendTxid, ds.sendErr
}

func (ds *dummySkycli) GetTransaction(txid string) (*webrpc.TxnResult, error) {
	ds.Lock()
	defer ds.Unlock()
	txjson := webrpc.TxnResult{
		Transaction: &visor.TransactionResult{},
	}
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
	log := testutil.NewLogger(t)
	dsc := newDummySkycli()
	dsc.sendTxid = "1111"
	s := NewService(Config{}, log, dsc)
	go func() {
		s.Run()
	}()

	addr := "KNtZkX2mw1UFuemv6FmEQxxhWCTWTm2Thk"
	sdr := NewSender(s)

	fmt.Println("=== Run\tTest send normal")
	time.AfterFunc(500*time.Millisecond, func() {
		dsc.changeConfirmStatus(true)
	})
	txid, err := sdr.Send(addr, 10, nil)
	require.Nil(t, err)
	require.Equal(t, "1111", txid)

	fmt.Println("=== Run\tTest send with time option")
	time.AfterFunc(time.Second, func() {
		dsc.changeConfirmStatus(true)
	})

	txid, err = sdr.Send(addr, 10, &SendOption{
		Timeout: 3 * time.Second,
	})
	require.Nil(t, err)
	require.Equal(t, "1111", txid)

	// test send coin failed
	fmt.Println("=== Run\tTest send failed")
	dsc.changeConfirmStatus(false)
	dsc.sendErr = errors.New("connect to node failed")
	time.AfterFunc(5*time.Second, func() {
		dsc.changeSendErr(nil)
		dsc.changeConfirmStatus(true)
	})

	txid, err = sdr.Send(addr, 20, nil)
	require.Nil(t, err)
	require.Equal(t, "1111", txid)

	// test get transaction failed
	fmt.Println("=== Run\ttest transaction falied")
	dsc.changeConfirmStatus(false)
	dsc.getTxErr = errors.New("get transaction failed")
	time.AfterFunc(5*time.Second, func() {
		dsc.changeGetTxErr(nil)
	})

	time.AfterFunc(7*time.Second, func() {
		dsc.changeConfirmStatus(true)
	})

	txid, err = sdr.Send(addr, 20, nil)
	require.Nil(t, err)
	require.Equal(t, "1111", txid)

	fmt.Println("=== Run\tTest invalid request address")
	txid, err = sdr.Send("invalid address", 20, nil)
	require.Contains(t, err.Error(), "Invalid request")
	require.Empty(t, txid)

	fmt.Println("=== Run\tTest send timeout")
	dsc.changeConfirmStatus(false)
	time.AfterFunc(4*time.Second, func() {
		dsc.changeConfirmStatus(true)
	})
	time.AfterFunc(time.Second, func() {
		_, err = sdr.Send(addr, 20, &SendOption{
			Timeout: time.Second * 3,
		})
		require.Equal(t, ErrSendBufferFull, err)
		s.Shutdown()
	})

	_, err = sdr.Send(addr, 10, nil)
	require.Nil(t, err)
}

func TestVerifyRequest(t *testing.T) {
	var testCases = []struct {
		name string
		req  Request
		err  bool
	}{
		{
			"valid address",
			Request{
				Address: "KNtZkX2mw1UFuemv6FmEQxxhWCTWTm2Thk",
				Coins:   1,
			},
			false,
		},
		{
			"invalid address",
			Request{
				Address: "addr1",
				Coins:   1,
			},
			true,
		},
		{
			"invalid coin amount",
			Request{
				Address: "KNtZkX2mw1UFuemv6FmEQxxhWCTWTm2Thk",
				Coins:   0,
			},
			true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := verifyRequest(tc.req)
			require.Equal(t, tc.err, err != nil)
		})
	}
}
