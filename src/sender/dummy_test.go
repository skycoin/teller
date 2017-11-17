package sender

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skycoin/teller/src/util/testutil"
)

func TestDummySender(t *testing.T) {
	log, _ := testutil.NewLogger(t)

	s := NewDummySender(log)

	addr := "2VZu3rZozQ6nN37YSdj3EZJV7wSFVuLSm2X"
	var coins uint64 = 100

	txn, err := s.CreateTransaction(addr, coins)
	require.NoError(t, err)
	require.NotNil(t, txn)
	require.Len(t, txn.Out, 1)

	require.Equal(t, addr, txn.Out[0].Address.String())
	require.Equal(t, coins, txn.Out[0].Coins)

	// Another txn with the same dest addr and coins should have a different txid
	txn2, err := s.CreateTransaction(addr, coins)
	require.NoError(t, err)
	require.NotEqual(t, txn.TxIDHex(), txn2.TxIDHex())

	bRsp := s.BroadcastTransaction(txn)
	require.NotNil(t, bRsp)
	require.NoError(t, bRsp.Err)
	require.Equal(t, txn.TxIDHex(), bRsp.Txid)

	// Broadcasting twice causes an error
	bRsp = s.BroadcastTransaction(txn)
	require.NotNil(t, bRsp)
	require.Error(t, bRsp.Err)
	require.Empty(t, bRsp.Txid)

	cRsp := s.IsTxConfirmed(txn.TxIDHex())
	require.NotNil(t, cRsp)
	require.NoError(t, cRsp.Err)
	require.False(t, cRsp.Confirmed)

	s.broadcastTxns[txn.TxIDHex()].Confirmed = true

	cRsp = s.IsTxConfirmed(txn.TxIDHex())
	require.NotNil(t, cRsp)
	require.NoError(t, cRsp.Err)
	require.True(t, cRsp.Confirmed)
}
