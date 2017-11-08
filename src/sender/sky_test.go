package sender

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/skycoin/skycoin/src/api/cli"
	"github.com/skycoin/skycoin/src/api/webrpc"
	"github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/skycoin/src/coin"
	"github.com/skycoin/skycoin/src/visor"
	"github.com/skycoin/teller/src/util/testutil"
	"github.com/stretchr/testify/require"
)

type dummySkycli struct {
	sync.Mutex
	broadcastTxTxid string
	broadcastTxErr  error
	createTxErr     error
	txConfirmed     bool
	getTxErr        error
}

func newDummySkycli() *dummySkycli {
	return &dummySkycli{}
}

func (ds *dummySkycli) BroadcastTransaction(tx *coin.Transaction) (string, error) {
	return ds.broadcastTxTxid, ds.broadcastTxErr
}

func (ds *dummySkycli) CreateTransaction(destAddr string, coins uint64) (*coin.Transaction, error) {
	if ds.createTxErr != nil {
		return nil, ds.createTxErr
	}

	return ds.createTransaction(destAddr, coins)
}

func (ds *dummySkycli) createTransaction(destAddr string, coins uint64) (*coin.Transaction, error) {
	addr, err := cipher.DecodeBase58Address(destAddr)
	if err != nil {
		return nil, err
	}

	return &coin.Transaction{
		Out: []coin.TransactionOutput{
			{
				Address: addr,
				Coins:   coins,
			},
		},
	}, nil
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

func (ds *dummySkycli) changeCreateTxErr(err error) {
	ds.Lock()
	defer ds.Unlock()
	ds.createTxErr = err
}

func (ds *dummySkycli) changeBroadcastTxErr(err error) {
	ds.Lock()
	defer ds.Unlock()
	ds.broadcastTxErr = err
}

func (ds *dummySkycli) changeBroadcastTxTxid(txid string) { // nolint: unparam
	ds.Lock()
	defer ds.Unlock()
	ds.broadcastTxTxid = txid
}

func (ds *dummySkycli) changeGetTxErr(err error) {
	ds.Lock()
	defer ds.Unlock()
	ds.getTxErr = err
}

func TestSenderBroadcastTransaction(t *testing.T) {
	log, _ := testutil.NewLogger(t)
	dsc := newDummySkycli()

	dsc.changeBroadcastTxTxid("1111")
	s := NewService(log, dsc)
	go func() {
		s.Run()
	}()

	addr := "KNtZkX2mw1UFuemv6FmEQxxhWCTWTm2Thk"
	sdr := NewRetrySender(s)

	broadcastTx := func(sender Sender, addr string, amt uint64) (string, error) {
		tx, err := sdr.CreateTransaction(addr, amt)
		if err != nil {
			return "", err
		}

		rsp := sdr.BroadcastTransaction(tx)
		require.NotNil(t, rsp)

		if rsp.Err != nil {
			return "", rsp.Err
		}

		return rsp.Txid, nil
	}

	t.Log("=== Run\tTest broadcastTx normal")
	time.AfterFunc(500*time.Millisecond, func() {
		dsc.changeConfirmStatus(true)
	})
	txid, err := broadcastTx(sdr, addr, 10)
	require.Nil(t, err)
	require.Equal(t, "1111", txid)

	// test broadcastTx coin failed
	t.Log("=== Run\tTest broadcastTx failed")
	dsc.changeConfirmStatus(false)
	dsc.changeBroadcastTxErr(errors.New("connect to node failed"))
	time.AfterFunc(5*time.Second, func() {
		dsc.changeBroadcastTxErr(nil)
		dsc.changeConfirmStatus(true)
	})

	txid, err = broadcastTx(sdr, addr, 20)
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

	txid, err = broadcastTx(sdr, addr, 20)
	require.Nil(t, err)
	require.Equal(t, "1111", txid)

	t.Log("=== Run\tTest invalid request address")
	txid, err = broadcastTx(sdr, "invalid address", 20)
	require.Equal(t, "Invalid address length", err.Error())
	require.Empty(t, txid)
}

func TestCreateTransactionVerify(t *testing.T) {
	var testCases = []struct {
		name       string
		sendAmount cli.SendAmount
		err        bool
	}{
		{
			"valid address",
			cli.SendAmount{
				Addr:  "KNtZkX2mw1UFuemv6FmEQxxhWCTWTm2Thk",
				Coins: 1,
			},
			false,
		},
		{
			"invalid address",
			cli.SendAmount{
				Addr:  "addr1",
				Coins: 1,
			},
			true,
		},
		{
			"invalid coin amount",
			cli.SendAmount{
				Addr:  "KNtZkX2mw1UFuemv6FmEQxxhWCTWTm2Thk",
				Coins: 0,
			},
			true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSendAmount(tc.sendAmount)
			require.Equal(t, tc.err, err != nil)
		})
	}
}
