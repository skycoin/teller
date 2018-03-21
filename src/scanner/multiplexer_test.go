package scanner

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/boltdb/bolt"
	"github.com/stretchr/testify/require"

	"github.com/MDLlife/teller/src/util/testutil"
)

var (
	ErrBtcScannerAlreadyExists   = fmt.Errorf("scanner of coinType %s already exists", CoinTypeBTC)
	ErrEthScannerAlreadyExists   = fmt.Errorf("scanner of coinType %s already exists", CoinTypeETH)
	ErrSKYScannerAlreadyExists   = fmt.Errorf("scanner of coinType %s already exists", CoinTypeSKY)
	ErrWAVESScannerAlreadyExists = fmt.Errorf("scanner of coinType %s already exists", CoinTypeWAVES)
	ErrNilScanner                = errors.New("nil scanner")
)

func testAddBtcScanAddresses(t *testing.T, m *Multiplexer) int64 {
	var nDeposits int64
	// This address has 0 deposits
	err := m.AddScanAddress("1LcEkgX8DCrQczLMVh9LDTRnkdVV2oun3A", CoinTypeBTC)
	require.NoError(t, err)
	nDeposits = nDeposits + 0

	// This address has:
	// 1 deposit, in block 235206
	// 1 deposit, in block 235207
	err = m.AddScanAddress("1N8G4JM8krsHLQZjC51R7ZgwDyihmgsQYA", CoinTypeBTC)
	require.NoError(t, err)
	nDeposits = nDeposits + 2

	// This address has:
	// 31 deposits in block 235205
	// 47 deposits in block 235206
	// 22 deposits, in block 235207
	// 26 deposits, in block 235214
	err = m.AddScanAddress("1LEkderht5M5yWj82M87bEd4XDBsczLkp9", CoinTypeBTC)
	require.NoError(t, err)
	nDeposits = nDeposits + 126

	return nDeposits
}

func testAddEthScanAddresses(t *testing.T, m *Multiplexer) int64 {
	var nDeposits int64

	// This address has 0 deposits
	err := m.AddScanAddress("0x2cf014d432e92685ef1cf7bc7967a4e4debca092", CoinTypeETH)
	require.NoError(t, err)
	nDeposits = nDeposits + 0

	// This address has:
	// 1 deposit, in block 2325212
	err = m.AddScanAddress("0x87b127ee022abcf9881b9bad6bb6aac25229dff0", CoinTypeETH)
	require.NoError(t, err)
	nDeposits = nDeposits + 2

	// This address has:
	// 9 deposits in block 2325213
	err = m.AddScanAddress("0xbfc39b6f805a9e40e77291aff27aee3c96915bdd", CoinTypeETH)
	require.NoError(t, err)
	nDeposits = nDeposits + 9

	return nDeposits
}

func testAddSKYScanAddresses(t *testing.T, m *Multiplexer) int64 {
	var nDeposits int64
	// This address has 0 deposits
	err := m.AddScanAddress("cBnu9sUvv12dovBmjQKTtfE4rbjMmf3fzW", CoinTypeSKY)
	require.NoError(t, err)
	nDeposits = nDeposits + 2

	// This address has:
	// 1 deposit, in block 235206
	// 1 deposit, in block 235207
	err = m.AddScanAddress("fyqX5YuwXMUs4GEUE3LjLyhrqvNztFHQ4B", CoinTypeSKY)
	require.NoError(t, err)
	nDeposits = nDeposits + 5

	return nDeposits
}

func testAddWAVESScanAddresses(t *testing.T, m *Multiplexer) int64 {
	var nDeposits int64
	// This address has 0 deposits
	err := m.AddScanAddress("3PRDjxHwETEhYXM3tjVMU4oYhfj3dqT6Vuw", CoinTypeWAVES)
	require.NoError(t, err)
	nDeposits = nDeposits + 2

	// This address has:
	// 1 deposit, in block 235206
	// 1 deposit, in block 235207
	err = m.AddScanAddress("3P9dUze9nHRdfoKhFrZYKdsSpwW9JoE6Mzf", CoinTypeWAVES)
	require.NoError(t, err)
	nDeposits = nDeposits + 3

	return nDeposits
}

func testAddBtcScanner(t *testing.T, db *bolt.DB, m *Multiplexer) (*BTCScanner, func()) {
	scr, shutdown := setupBtcScanner(t, db)
	err := m.AddScanner(scr, CoinTypeBTC)
	require.NoError(t, err)
	count := m.GetScannerCount()

	//add btc again, should be error
	err = m.AddScanner(scr, CoinTypeBTC)
	require.Equal(t, ErrBtcScannerAlreadyExists, err)
	//scanner count no change
	require.Equal(t, count, m.GetScannerCount())

	//add wrong scanner
	err = m.AddScanner(nil, CoinTypeBTC)
	require.Equal(t, ErrNilScanner, err)
	return scr, shutdown
}

func testAddEthScanner(t *testing.T, db *bolt.DB, m *Multiplexer) (*ETHScanner, func()) {
	ethscr, ethshutdown := setupEthScanner(t, db)

	err := m.AddScanner(ethscr, CoinTypeETH)
	require.NoError(t, err)
	//add eth again
	err = m.AddScanner(ethscr, CoinTypeETH)
	require.Equal(t, ErrEthScannerAlreadyExists, err)
	return ethscr, ethshutdown
}

func testAddSKYScanner(t *testing.T, db *bolt.DB, m *Multiplexer) (*SKYScanner, func()) {
	scr, shutdown := setupSkyScanner(t, db)
	err := m.AddScanner(scr, CoinTypeSKY)
	require.NoError(t, err)
	count := m.GetScannerCount()

	//add btc again, should be error
	err = m.AddScanner(scr, CoinTypeSKY)
	require.Equal(t, ErrSKYScannerAlreadyExists, err)
	//scanner count no change
	require.Equal(t, count, m.GetScannerCount())

	//add wrong scanner
	err = m.AddScanner(nil, CoinTypeSKY)
	require.Equal(t, ErrNilScanner, err)
	return scr, shutdown
}

func testAddWAVESScanner(t *testing.T, db *bolt.DB, m *Multiplexer) (*WAVESScanner, func()) {
	scr, shutdown := setupWavesScanner(t, db)
	err := m.AddScanner(scr, CoinTypeWAVES)
	require.NoError(t, err)
	count := m.GetScannerCount()

	//add btc again, should be error
	err = m.AddScanner(scr, CoinTypeWAVES)
	require.Equal(t, ErrWAVESScannerAlreadyExists, err)
	//scanner count no change
	require.Equal(t, count, m.GetScannerCount())

	//add wrong scanner
	err = m.AddScanner(nil, CoinTypeWAVES)
	require.Equal(t, ErrNilScanner, err)
	return scr, shutdown
}

func TestMultiplexerOnlyBtc(t *testing.T) {
	//init btc db
	btcDB := openDummyBtcDB(t)
	defer testutil.CheckError(t, btcDB.Close)

	//create logger
	log, _ := testutil.NewLogger(t)
	//create multiplexer
	m := NewMultiplexer(log)

	//add btc scanner to multiplexer
	scr, shutdown := testAddBtcScanner(t, btcDB, m)
	defer shutdown()

	nDeposits := testAddBtcScanAddresses(t, m)

	go testutil.CheckError(t, m.Multiplex)

	done := make(chan struct{})
	go func() {
		defer close(done)
		var dvs []DepositNote
		for dv := range m.GetDeposit() {
			dvs = append(dvs, dv)
			dv.ErrC <- nil
		}

		require.Equal(t, nDeposits, int64(len(dvs)))
	}()

	// Wait for at least twice as long as the number of deposits to process
	// If there are few deposits, wait at least 5 seconds
	// This only needs to wait at least 1 second normally, but if testing
	// with -race, it needs to wait 5.
	shutdownWait := time.Duration(int64(scr.Base.(*BaseScanner).Cfg.ScanPeriod) * nDeposits * 3)
	if shutdownWait < minShutdownWait {
		shutdownWait = minShutdownWait
	}

	time.AfterFunc(shutdownWait, func() {
		scr.Shutdown()
		m.Shutdown()
	})
	err := scr.Run()
	require.NoError(t, err)
	<-done
}

func TestMultiplexerOnlySKY(t *testing.T) {
	//init sky db
	skyDB := openDummySkyDB(t)
	defer testutil.CheckError(t, skyDB.Close)

	//create logger
	log, _ := testutil.NewLogger(t)
	//create multiplexer
	m := NewMultiplexer(log)

	//add sky scanner to multiplexer
	scr, shutdown := testAddSKYScanner(t, skyDB, m)
	defer shutdown()

	nDeposits := testAddSKYScanAddresses(t, m)

	go testutil.CheckError(t, m.Multiplex)

	done := make(chan struct{})
	go func() {
		defer close(done)
		var dvs []DepositNote
		for dv := range m.GetDeposit() {
			dvs = append(dvs, dv)
			dv.ErrC <- nil
		}

		require.Equal(t, nDeposits, int64(len(dvs)))
	}()

	// Wait for at least twice as long as the number of deposits to process
	// If there are few deposits, wait at least 5 seconds
	// This only needs to wait at least 1 second normally, but if testing
	// with -race, it needs to wait 5.
	shutdownWait := time.Duration(int64(scr.Base.(*BaseScanner).Cfg.ScanPeriod) * nDeposits * 3)
	if shutdownWait < minShutdownWait {
		shutdownWait = minShutdownWait
	}

	time.AfterFunc(shutdownWait, func() {
		scr.Shutdown()
		m.Shutdown()
	})
	err := scr.Run()
	require.NoError(t, err)
	<-done
}

func TestMultiplexerOnlyWAVES(t *testing.T) {
	//init sky db
	wavesDB := openDummyWavesDB(t)
	defer testutil.CheckError(t, wavesDB.Close)

	//create logger
	log, _ := testutil.NewLogger(t)
	//create multiplexer
	m := NewMultiplexer(log)

	//add sky scanner to multiplexer
	scr, shutdown := testAddWAVESScanner(t, wavesDB, m)
	defer shutdown()

	nDeposits := testAddWAVESScanAddresses(t, m)

	go testutil.CheckError(t, m.Multiplex)

	done := make(chan struct{})
	go func() {
		defer close(done)
		var dvs []DepositNote
		for dv := range m.GetDeposit() {
			dvs = append(dvs, dv)
			dv.ErrC <- nil
		}

		require.True(t, nDeposits <= int64(len(dvs)))
	}()

	// Wait for at least twice as long as the number of deposits to process
	// If there are few deposits, wait at least 5 seconds
	// This only needs to wait at least 1 second normally, but if testing
	// with -race, it needs to wait 5.
	shutdownWait := time.Duration(int64(scr.Base.(*BaseScanner).Cfg.ScanPeriod) * nDeposits * 3)
	if shutdownWait < minShutdownWait {
		shutdownWait = minShutdownWait
	}

	time.AfterFunc(shutdownWait, func() {
		scr.Shutdown()
		m.Shutdown()
	})
	err := scr.Run()
	require.NoError(t, err)
	<-done
}

func TestMultiplexerForAll(t *testing.T) {
	//init btc db
	btcDB := openDummyBtcDB(t)
	defer testutil.CheckError(t, btcDB.Close)

	ethDB := openDummyEthDB(t)
	defer testutil.CheckError(t, ethDB.Close)

	skyDB := openDummySkyDB(t)
	defer testutil.CheckError(t, skyDB.Close)

	wavesDB := openDummyWavesDB(t)
	defer testutil.CheckError(t, wavesDB.Close)

	//create logger
	log, _ := testutil.NewLogger(t)
	//create multiplexer
	m := NewMultiplexer(log)

	//add btc scanner to multiplexer
	scr, shutdown := testAddBtcScanner(t, btcDB, m)
	defer shutdown()

	//add eth scanner to multiplexer
	ethscr, ethshutdown := testAddEthScanner(t, ethDB, m)
	defer ethshutdown()

	//add SKY scanner to multiplexer
	skyscr, skyshutdown := testAddSKYScanner(t, skyDB, m)
	defer skyshutdown()

	//add WAVES scanner to multiplexer
	wavesscr, wavesshutdown := testAddWAVESScanner(t, wavesDB, m)
	defer wavesshutdown()

	// 4 scanner in multiplexer
	require.Equal(t, 4, m.GetScannerCount())

	nDepositsBtc := testAddBtcScanAddresses(t, m)
	nDepositsEth := testAddEthScanAddresses(t, m)
	nDepositsSKY := testAddSKYScanAddresses(t, m)
	nDepositsSWAVES := testAddWAVESScanAddresses(t, m)

	go func() {
		err := m.Multiplex()
		require.NoError(t, err)
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		var dvs []DepositNote
		var wavesDeposits []DepositNote
		for dv := range m.GetDeposit() {
			// separate waves from others, waves is doing real http
			// results may vary if scanner manages to scan more blocks in less time
			if dv.Deposit.CoinType == CoinTypeWAVES {
				wavesDeposits = append(wavesDeposits, dv)
			} else {
				dvs = append(dvs, dv)
			}
			dv.ErrC <- nil
		}

		require.Equal(t, nDepositsBtc+nDepositsEth+nDepositsSKY, int64(len(dvs)))
		require.True(t, nDepositsSWAVES <= int64(len(wavesDeposits)))
	}()

	// Wait for at least twice as long as the number of deposits to process
	// If there are few deposits, wait at least 5 seconds
	// This only needs to wait at least 1 second normally, but if testing
	// with -race, it needs to wait 5.
	shutdownWait := time.Duration(int64(scr.Base.(*BaseScanner).Cfg.ScanPeriod) * (nDepositsBtc + nDepositsEth + nDepositsSKY + nDepositsSWAVES) * 3)
	if shutdownWait < minShutdownWait {
		shutdownWait = minShutdownWait
	}

	time.AfterFunc(shutdownWait, func() {
		wavesscr.Shutdown()
		skyscr.Shutdown()
		ethscr.Shutdown()
		scr.Shutdown()
		m.Shutdown()
	})
	go func() {
		err := ethscr.Run()
		require.NoError(t, err)
	}()
	go func() {
		err := skyscr.Run()
		require.NoError(t, err)
	}()
	go func() {
		err := wavesscr.Run()
		require.NoError(t, err)
	}()
	go func() {
		err := scr.Run()
		require.NoError(t, err)
	}()
	<-done
}
