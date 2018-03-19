package sender

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/MDLlife/MDL/src/api/cli"
	"github.com/MDLlife/MDL/src/api/webrpc"
	"github.com/MDLlife/MDL/src/cipher"
	"github.com/MDLlife/MDL/src/coin"
	"github.com/MDLlife/MDL/src/visor"

	"github.com/MDLlife/teller/src/util/testutil"
)

type dummyMDLClient struct {
	sync.Mutex
	broadcastTxTxid string
	broadcastTxErr  error
	createTxErr     error
	txConfirmed     bool
	getTxErr        error
}

func newDummyMDLClient() *dummyMDLClient {
	return &dummyMDLClient{}
}

func (ds *dummyMDLClient) BroadcastTransaction(tx *coin.Transaction) (string, error) {
	ds.Lock()
	defer ds.Unlock()
	return ds.broadcastTxTxid, ds.broadcastTxErr
}

func (ds *dummyMDLClient) CreateTransaction(destAddr string, coins uint64) (*coin.Transaction, error) {
	ds.Lock()
	defer ds.Unlock()

	if ds.createTxErr != nil {
		return nil, ds.createTxErr
	}

	return ds.createTransaction(destAddr, coins)
}

func (ds *dummyMDLClient) createTransaction(destAddr string, coins uint64) (*coin.Transaction, error) {
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

func (ds *dummyMDLClient) GetTransaction(txid string) (*webrpc.TxnResult, error) {
	ds.Lock()
	defer ds.Unlock()
	txjson := webrpc.TxnResult{
		Transaction: &visor.TransactionResult{},
	}
	txjson.Transaction.Status.Confirmed = ds.txConfirmed
	return &txjson, ds.getTxErr
}

func (ds *dummyMDLClient) Balance() (*cli.Balance, error) {
	return &cli.Balance{
		Coins: "100.000000",
		Hours: "100",
	}, nil
}

func (ds *dummyMDLClient) changeConfirmStatus(v bool) {
	ds.Lock()
	defer ds.Unlock()
	ds.txConfirmed = v
}

func (ds *dummyMDLClient) changeBroadcastTxErr(err error) {
	ds.Lock()
	defer ds.Unlock()
	ds.broadcastTxErr = err
}

func (ds *dummyMDLClient) changeBroadcastTxTxid(txid string) { // nolint: unparam
	ds.Lock()
	defer ds.Unlock()
	ds.broadcastTxTxid = txid
}

func (ds *dummyMDLClient) changeGetTxErr(err error) {
	ds.Lock()
	defer ds.Unlock()
	ds.getTxErr = err
}

func TestSenderBroadcastTransaction(t *testing.T) {
	log, _ := testutil.NewLogger(t)
	dsc := newDummyMDLClient()

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
