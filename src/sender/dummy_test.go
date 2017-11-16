package sender

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skycoin/teller/src/util/testutil"
)

func TestDummySender(t *testing.T) {
	log, _ := testutil.NewLogger(t)

	s := NewDummySender(log, "")

	addr := "2VZu3rZozQ6nN37YSdj3EZJV7wSFVuLSm2X"
	var amt uint64 = 100

	txn, err := s.CreateTransaction(addr, amt)
	require.NoError(t, err)
	require.NotNil(t, txn)
	require.Len(t, txn.Out, 1)

	require.Equal(t, addr, txn.Out[0].Address.String())
	require.Equal(t, amt, txn.Out[0].Coins)

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
