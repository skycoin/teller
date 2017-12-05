package scanner

import (
	"testing"

	"github.com/skycoin/teller/src/scanner"
	"github.com/stretchr/testify/require"
)

func TestMultiplexer(t *testing.T) {
	btcDB := openDummyBtcDB(t)
	defer btcDB.Close()
	scr, shutdown := setupScanner(t, btcDB)
	defer shutdown()
	m := NewMultiplexer()
	err := m.AddScanner(scr, scanner.CoinTypeBTC)
	require.NoError(t, err)
	require.Equal(t, 1, m.GetScannerCount())
	err = m.AddScanner(scr, scanner.CoinTypeBTC)
	require.Error(t, err)
	require.Equal(t, 1, m.GetScannerCount())

	ethDB := openDummyEthDB(t)
	ethscr, ethshutdown := setupEthScanner(t, ethDB)
	defer ethDB.Close()
	defer ethshutdown()

	err = m.AddScanner(ethscr, scanner.CoinTypeETH)
	require.NoError(t, err)
	require.Equal(t, 2, m.GetScannerCount())
	err = m.AddScanner(ethscr, scanner.CoinTypeETH)
	require.Error(t, err)
	require.Equal(t, 2, m.GetScannerCount())

}
