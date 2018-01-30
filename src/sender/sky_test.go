package sender

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skycoin/skycoin/src/api/cli"
	"github.com/skycoin/skycoin/src/api/webrpc"
	"github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/skycoin/src/coin"
	"github.com/skycoin/skycoin/src/visor"

	"github.com/skycoin/teller/src/util/testutil"
)

type dummySkyClient struct {
	sync.Mutex
	broadcastTxTxid string
	broadcastTxErr  error
	createTxErr     error
	txConfirmed     bool
	getTxErr        error
}

func newDummySkyClient() *dummySkyClient {
	return &dummySkyClient{}
}

func (ds *dummySkyClient) BroadcastTransaction(tx *coin.Transaction) (string, error) {
	return ds.broadcastTxTxid, ds.broadcastTxErr
}

func (ds *dummySkyClient) CreateTransaction(destAddr string, coins uint64) (*coin.Transaction, error) {
	if ds.createTxErr != nil {
		return nil, ds.createTxErr
	}

	return ds.createTransaction(destAddr, coins)
}

func (ds *dummySkyClient) createTransaction(destAddr string, coins uint64) (*coin.Transaction, error) {
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

func (ds *dummySkyClient) GetTransaction(txid string) (*webrpc.TxnResult, error) {
	ds.Lock()
	defer ds.Unlock()
	txjson := webrpc.TxnResult{
		Transaction: &visor.TransactionResult{},
	}
	txjson.Transaction.Status.Confirmed = ds.txConfirmed
	return &txjson, ds.getTxErr
}

func (ds *dummySkyClient) Balance() (*cli.Balance, error) {
	return &cli.Balance{
		Coins: "100.000000",
		Hours: "100",
	}, nil
}

func (ds *dummySkyClient) changeConfirmStatus(v bool) {
	ds.Lock()
	defer ds.Unlock()
	ds.txConfirmed = v
}

func (ds *dummySkyClient) changeBroadcastTxErr(err error) {
	ds.Lock()
	defer ds.Unlock()
	ds.broadcastTxErr = err
}

func (ds *dummySkyClient) changeBroadcastTxTxid(txid string) { // nolint: unparam
	ds.Lock()
	defer ds.Unlock()
	ds.broadcastTxTxid = txid
}

func (ds *dummySkyClient) changeGetTxErr(err error) {
	ds.Lock()
	defer ds.Unlock()
	ds.getTxErr = err
}

func TestSenderBroadcastTransaction(t *testing.T) {
	log, _ := testutil.NewLogger(t)
	dsc := newDummySkyClient()

	dsc.changeBroadcastTxTxid("1111")
	s := NewService(log, dsc)
	go func() {
		err := s.Run()
		require.NoError(t, err)
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
	require.Equal(t, "Invalid base58 character", err.Error())
	require.Empty(t, txid)

	t.Log("=== Run\tTest invalid request address 2")
	txid, err = broadcastTx(sdr, " bxpUG8sCjeT6X1ES5SbD2LZrRudqiTY7wx", 20)
	require.Equal(t, "Invalid base58 character", err.Error())
	require.Empty(t, txid)

	t.Log("=== Run\tTest invalid request address 3")
	txid, err = broadcastTx(sdr, "bxpUG8sCjeT6X1ES5SbD2LZrRudqiTY7wxx", 20)
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
