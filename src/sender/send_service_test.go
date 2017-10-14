package sender

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/skycoin/skycoin/src/api/webrpc"
	"github.com/skycoin/skycoin/src/visor"
	"github.com/skycoin/teller/src/util/testutil"
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
	sdr := NewRetrySender(s)

	send := func(sender Sender, addr string, amt uint64) (string, error) {
		rsp := sdr.Send(addr, amt)
		require.NotNil(t, rsp)

		if rsp.Err != nil {
			return "", rsp.Err
		}

		return rsp.Txid, nil
	}

	t.Log("=== Run\tTest send normal")
	time.AfterFunc(500*time.Millisecond, func() {
		dsc.changeConfirmStatus(true)
	})
	txid, err := send(sdr, addr, 10)
	require.Nil(t, err)
	require.Equal(t, "1111", txid)

	// test send coin failed
	t.Log("=== Run\tTest send failed")
	dsc.changeConfirmStatus(false)
	dsc.sendErr = errors.New("connect to node failed")
	time.AfterFunc(5*time.Second, func() {
		dsc.changeSendErr(nil)
		dsc.changeConfirmStatus(true)
	})

	txid, err = send(sdr, addr, 20)
	require.Nil(t, err)
	require.Equal(t, "1111", txid)

	// test get transaction failed
	t.Log("=== Run\ttest transaction falied")
	dsc.changeConfirmStatus(false)
	dsc.getTxErr = errors.New("get transaction failed")
	time.AfterFunc(5*time.Second, func() {
		dsc.changeGetTxErr(nil)
	})

	time.AfterFunc(7*time.Second, func() {
		dsc.changeConfirmStatus(true)
	})

	txid, err = send(sdr, addr, 20)
	require.Nil(t, err)
	require.Equal(t, "1111", txid)

	t.Log("=== Run\tTest invalid request address")
	txid, err = send(sdr, "invalid address", 20)
	require.Contains(t, err.Error(), "Invalid request")
	require.Empty(t, txid)
}

func TestVerifyRequest(t *testing.T) {
	var testCases = []struct {
		name string
		req  SendRequest
		err  bool
	}{
		{
			"valid address",
			SendRequest{
				Address: "KNtZkX2mw1UFuemv6FmEQxxhWCTWTm2Thk",
				Coins:   1,
			},
			false,
		},
		{
			"invalid address",
			SendRequest{
				Address: "addr1",
				Coins:   1,
			},
			true,
		},
		{
			"invalid coin amount",
			SendRequest{
				Address: "KNtZkX2mw1UFuemv6FmEQxxhWCTWTm2Thk",
				Coins:   0,
			},
			true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Verify()
			require.Equal(t, tc.err, err != nil)
		})
	}
}
