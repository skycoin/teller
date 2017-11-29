package scanner

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/boltdb/bolt"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/stretchr/testify/require"

	"github.com/skycoin/teller/src/util/dbutil"
	"github.com/skycoin/teller/src/util/testutil"
)

const (
	// run tests in parallel
	parallel        = true
	minShutdownWait = time.Second // set to time.Second * 5 when using -race
)

var (
	dummyBlocksBktName = []byte("blocks")

	errNoBlockHash = errors.New("no block hash found for height")
)

type scannedBlock struct {
	Hash   string
	Height int32
}

type dummyBtcrpcclient struct {
	db                           *bolt.DB
	blockHashes                  map[int64]string
	blockCount                   int64
	blockCountError              error
	blockVerboseTxError          error
	blockVerboseTxErrorCallCount int
	blockVerboseTxCallCount      int

	// used for testScannerBlockNextHashAppears
	blockNextHashMissingOnceAt int64
	hasSetMissingHash          bool
}

func openDummyBtcDB(t *testing.T) *bolt.DB {
	// Blocks 235205 through 235214 are stored in this DB
	db, err := bolt.Open("./btc.db", 0600, nil)
	require.NoError(t, err)
	return db
}

func newDummyBtcrpcclient(db *bolt.DB) *dummyBtcrpcclient {
	return &dummyBtcrpcclient{
		db:          db,
		blockHashes: make(map[int64]string),
	}
}

func (dbc *dummyBtcrpcclient) Shutdown() {
}

func (dbc *dummyBtcrpcclient) GetBlockVerbose(hash *chainhash.Hash) (*btcjson.GetBlockVerboseResult, error) {
	return dbc.GetBlockVerboseTx(hash)
}
func (dbc *dummyBtcrpcclient) GetBlockVerboseTx(hash *chainhash.Hash) (*btcjson.GetBlockVerboseResult, error) {
	dbc.blockVerboseTxCallCount++
	if dbc.blockVerboseTxCallCount == dbc.blockVerboseTxErrorCallCount {
		return nil, dbc.blockVerboseTxError
	}

	var block *btcjson.GetBlockVerboseResult
	if err := dbc.db.View(func(tx *bolt.Tx) error {
		var b btcjson.GetBlockVerboseResult
		v := tx.Bucket(dummyBlocksBktName).Get([]byte(hash.String()))
		if v == nil {
			return fmt.Errorf("no block found in db with hash %s", hash.String())
		}

		if err := json.Unmarshal(v, &b); err != nil {
			return err
		}

		block = &b
		return nil
	}); err != nil {
		return nil, err
	}

	if block.Height == dbc.blockCount {
		block.NextHash = ""
	} else if block.Height == dbc.blockNextHashMissingOnceAt && !dbc.hasSetMissingHash {
		dbc.hasSetMissingHash = true
		block.NextHash = ""
	}

	if block.Height > dbc.blockCount {
		panic("scanner should not be scanning blocks past the blockCount height")
	}

	return block, nil
}

func (dbc *dummyBtcrpcclient) GetBlockCount() (int64, error) {
	if dbc.blockCountError != nil {
		// blockCountError is only returned once
		err := dbc.blockCountError
		dbc.blockCountError = nil
		return 0, err
	}

	return dbc.blockCount, nil
}

func (dbc *dummyBtcrpcclient) GetBlockHash(height int64) (*chainhash.Hash, error) {
	hash := dbc.blockHashes[height]
	if hash == "" {
		return nil, errNoBlockHash
	}

	return chainhash.NewHashFromStr(hash)
}

func setupScannerWithDB(t *testing.T, btcDB *bolt.DB, db *bolt.DB) *BTCScanner {
	log, _ := testutil.NewLogger(t)

	// Blocks 235205 through 235214 are stored in btc.db
	// Refer to https://blockchain.info or another explorer to see the block data
	rpc := newDummyBtcrpcclient(btcDB)

	// The hash of the initial scan block needs to be set. The others don't
	// need to be, since the scanner follows block.NextHash to find the rest
	rpc.blockHashes[235205] = "000000000000018d8ece83a004c5a919210d67798d13aa901c4d07f8bf87b719"

	// 235214 is the highest block in the test data btc.db
	rpc.blockCount = 235214

	store, err := NewStore(log, db)
	require.NoError(t, err)

	cfg := Config{
		ScanPeriod:            time.Millisecond * 10,
		DepositBufferSize:     5,
		InitialScanHeight:     235205,
		ConfirmationsRequired: 0,
	}
	scr, err := NewBTCScanner(log, store, rpc, cfg)
	require.NoError(t, err)

	return scr
}

func setupScanner(t *testing.T, btcDB *bolt.DB) (*BTCScanner, func()) {
	db, shutdown := testutil.PrepareDB(t)

	scr := setupScannerWithDB(t, btcDB, db)

	return scr, shutdown
}

func testScannerRunProcessedLoop(t *testing.T, scr *BTCScanner, nDeposits int) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		var dvs []DepositNote
		for dv := range scr.GetDeposit() {
			dvs = append(dvs, dv)
			dv.ErrC <- nil
		}

		require.Equal(t, nDeposits, len(dvs))

		// check all deposits
		err := scr.store.(*BTCStore).db.View(func(tx *bolt.Tx) error {
			for _, dv := range dvs {
				var d Deposit
				err := dbutil.GetBucketObject(tx, depositBkt, dv.ID(), &d)
				require.NoError(t, err)
				if err != nil {
					return err
				}

				require.True(t, d.Processed)
				require.Equal(t, CoinTypeBTC, d.CoinType)
				require.NotEmpty(t, d.Address)
				require.NotEmpty(t, d.Value)
				require.NotEmpty(t, d.Height)
				require.NotEmpty(t, d.Tx)
			}

			return nil
		})
		require.NoError(t, err)
	}()

	// Wait for at least twice as long as the number of deposits to process
	// If there are few deposits, wait at least 5 seconds
	// This only needs to wait at least 1 second normally, but if testing
	// with -race, it needs to wait 5.
	shutdownWait := scr.cfg.ScanPeriod * time.Duration(nDeposits*2)
	if shutdownWait < minShutdownWait {
		shutdownWait = minShutdownWait
	}

	time.AfterFunc(shutdownWait, func() {
		scr.Shutdown()
	})

	err := scr.Run()
	require.NoError(t, err)
	<-done
}

func testScannerRun(t *testing.T, scr *BTCScanner) {
	nDeposits := 0

	// This address has 0 deposits
	err := scr.AddScanAddress("1LcEkgX8DCrQczLMVh9LDTRnkdVV2oun3A")
	require.NoError(t, err)
	nDeposits = nDeposits + 0

	// This address has:
	// 1 deposit, in block 235206
	// 1 deposit, in block 235207
	err = scr.AddScanAddress("1N8G4JM8krsHLQZjC51R7ZgwDyihmgsQYA")
	require.NoError(t, err)
	nDeposits = nDeposits + 2

	// This address has:
	// 31 deposits in block 235205
	// 47 deposits in block 235206
	// 22 deposits, in block 235207
	// 26 deposits, in block 235214
	err = scr.AddScanAddress("1LEkderht5M5yWj82M87bEd4XDBsczLkp9")
	require.NoError(t, err)
	nDeposits = nDeposits + 126

	// Make sure that the deposit buffer size is less than the number of deposits,
	// to test what happens when the buffer is full
	require.True(t, scr.cfg.DepositBufferSize < nDeposits)

	testScannerRunProcessedLoop(t, scr, nDeposits)
}

func testScannerRunProcessDeposits(t *testing.T, btcDB *bolt.DB) {
	// Tests that the scanner will scan multiple blocks sequentially, finding
	// all relevant deposits and adding them to the depositC channel.
	// All deposits on the depositC channel will be successfully processed
	// by the channel reader, and the scanner will mark these deposits as
	// "processed".
	scr, shutdown := setupScanner(t, btcDB)
	defer shutdown()

	testScannerRun(t, scr)
}

func testScannerGetBlockCountErrorRetry(t *testing.T, btcDB *bolt.DB) {
	// Test that if the scanner scan loop encounters an error when calling
	// GetBlockCount(), the loop continues to work fine
	// This test is that same as testScannerRunProcessDeposits,
	// except that the dummyBtcrpcclient is configured to return an error
	// from GetBlockCount() one time
	scr, shutdown := setupScanner(t, btcDB)
	defer shutdown()

	scr.btcClient.(*dummyBtcrpcclient).blockCountError = errors.New("block count error")

	testScannerRun(t, scr)
}

func testScannerConfirmationsRequired(t *testing.T, btcDB *bolt.DB) {
	// Test that the scanner uses cfg.ConfirmationsRequired correctly
	scr, shutdown := setupScanner(t, btcDB)
	defer shutdown()

	// Scanning starts at block 23505, set the blockCount height to 2
	// confirmations higher, so that only block 23505 is processed.
	scr.cfg.ConfirmationsRequired = 2
	scr.btcClient.(*dummyBtcrpcclient).blockCount = 235208

	// Add scan addresses for blocks 235205-235214, but only expect to scan
	// deposits from block 23505, since 235206 and 235207 don't have enough
	// confirmations
	nDeposits := 0

	// This address has:
	// 31 deposits in block 235205
	// 47 deposits in block 235206
	// 22 deposits, in block 235207
	// 26 deposits, in block 235214
	// Only blocks 235205 and 235206 are processed, because blockCount is set
	// to 235208 and the confirmations required is set to 2
	err := scr.AddScanAddress("1LEkderht5M5yWj82M87bEd4XDBsczLkp9")
	require.NoError(t, err)
	nDeposits = nDeposits + 78

	// Make sure that the deposit buffer size is less than the number of deposits,
	// to test what happens when the buffer is full
	require.True(t, scr.cfg.DepositBufferSize < nDeposits)

	testScannerRunProcessedLoop(t, scr, nDeposits)
}

func testScannerScanBlockFailureRetry(t *testing.T, btcDB *bolt.DB) {
	// Test that when scanBlock() fails, it logs "Scan block failed"
	// and retries scan of the same block after ScanPeriod elapses.
	scr, shutdown := setupScanner(t, btcDB)
	defer shutdown()

	// Return an error on the 2nd call to GetBlockVerboseTx
	scr.btcClient.(*dummyBtcrpcclient).blockVerboseTxError = errors.New("get block verbose tx error")
	scr.btcClient.(*dummyBtcrpcclient).blockVerboseTxErrorCallCount = 2

	testScannerRun(t, scr)
}

func testScannerBlockNextHashAppears(t *testing.T, btcDB *bolt.DB) {
	// Test that when a block has no NextHash, the scanner waits until it has
	// one, then resumes normally
	scr, shutdown := setupScanner(t, btcDB)
	defer shutdown()

	// The block at height 235208 will lack a NextHash one time
	// The scanner will continue and process everything normally
	scr.btcClient.(*dummyBtcrpcclient).blockNextHashMissingOnceAt = 235208

	testScannerRun(t, scr)
}

func testScannerDuplicateDepositScans(t *testing.T, btcDB *bolt.DB) {
	// Test that rescanning the same blocks doesn't send extra deposits
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	nDeposits := 0

	// This address has:
	// 1 deposit, in block 235206
	// 1 deposit, in block 235207
	scr := setupScannerWithDB(t, btcDB, db)
	err := scr.AddScanAddress("1N8G4JM8krsHLQZjC51R7ZgwDyihmgsQYA")
	require.NoError(t, err)
	nDeposits = nDeposits + 2

	testScannerRunProcessedLoop(t, scr, nDeposits)

	// Scanning again will have no new deposits
	scr = setupScannerWithDB(t, btcDB, db)
	testScannerRunProcessedLoop(t, scr, 0)
}

func testScannerLoadUnprocessedDeposits(t *testing.T, btcDB *bolt.DB) {
	// Test that pending unprocessed deposits from the db are loaded when
	// then scanner starts.
	scr, shutdown := setupScanner(t, btcDB)
	defer shutdown()

	// NOTE: This data is fake, but the addresses and Txid are valid
	unprocessedDeposits := []Deposit{
		{
			CoinType:  CoinTypeBTC,
			Address:   "1LEkderht5M5yWj82M87bEd4XDBsczLkp9",
			Value:     1e8,
			Height:    23505,
			Tx:        "239e007dc20805add047d305cdfb87de1bae9bea1e47acbf58f38731ad58d70d",
			N:         1,
			Processed: false,
		},
		{
			CoinType:  CoinTypeBTC,
			Address:   "16Lr3Zhjjb7KxeDxGPUrh3DMo29Lstif7j",
			Value:     10e8,
			Height:    23505,
			Tx:        "bf41a5352b6d59a401cd946432117b25fd5fc43186aef5cbbe3170c40050d104",
			N:         1,
			Processed: false,
		},
	}

	processedDeposit := Deposit{
		CoinType:  CoinTypeBTC,
		Address:   "1GH9ukgyetEJoWQFwUUeLcWQ8UgVgipLKb",
		Value:     100e8,
		Height:    23517,
		Tx:        "d61be86942d69dc7ba6d49c817957ecd0918798f030c73739206e6f48fe2a7c5",
		N:         1,
		Processed: true,
	}

	err := scr.store.(*BTCStore).db.Update(func(tx *bolt.Tx) error {
		for _, d := range unprocessedDeposits {
			if err := scr.store.(*BTCStore).pushDepositTx(tx, d); err != nil {
				require.NoError(t, err)
				return err
			}
		}

		// Add a processed deposit to make sure that processed deposits are filtered
		return scr.store.(*BTCStore).pushDepositTx(tx, processedDeposit)
	})
	require.NoError(t, err)

	// Don't add any watch addresses,
	// only process the unprocessed deposits from the backlog
	testScannerRunProcessedLoop(t, scr, len(unprocessedDeposits))
}

func testScannerProcessDepositError(t *testing.T, btcDB *bolt.DB) {
	// Test that when processDeposit() fails, the deposit is NOT marked as processed
	scr, shutdown := setupScanner(t, btcDB)
	defer shutdown()

	nDeposits := 0

	// This address has:
	// 31 deposits in block 235205
	// 47 deposits in block 235206
	// 22 deposits, in block 235207
	// 26 deposits, in block 235214
	err := scr.AddScanAddress("1LEkderht5M5yWj82M87bEd4XDBsczLkp9")
	require.NoError(t, err)
	nDeposits = nDeposits + 126

	// Make sure that the deposit buffer size is less than the number of deposits,
	// to test what happens when the buffer is full
	require.True(t, scr.cfg.DepositBufferSize < nDeposits)

	done := make(chan struct{})
	go func() {
		defer close(done)
		var dvs []DepositNote
		for dv := range scr.GetDeposit() {
			dvs = append(dvs, dv)
			dv.ErrC <- errors.New("failed to process deposit")
		}

		require.Equal(t, nDeposits, len(dvs))

		// check all deposits, none should be marked as "Processed"
		err := scr.store.(*BTCStore).db.View(func(tx *bolt.Tx) error {
			for _, dv := range dvs {
				var d Deposit
				err := dbutil.GetBucketObject(tx, depositBkt, dv.ID(), &d)
				require.NoError(t, err)
				if err != nil {
					return err
				}

				require.False(t, d.Processed)
				require.Equal(t, CoinTypeBTC, d.CoinType)
				require.Equal(t, "1LEkderht5M5yWj82M87bEd4XDBsczLkp9", d.Address)
				require.NotEmpty(t, d.Value)
				require.NotEmpty(t, d.Height)
				require.NotEmpty(t, d.Tx)
			}

			return nil
		})
		require.NoError(t, err)
	}()

	// Wait for at least twice as long as the number of deposits to process
	// If there are few deposits, wait at least 5 seconds
	// This only needs to wait at least 1 second normally, but if testing
	// with -race, it needs to wait 5.
	shutdownWait := scr.cfg.ScanPeriod * time.Duration(nDeposits*2)
	if shutdownWait < minShutdownWait {
		shutdownWait = minShutdownWait
	}

	time.AfterFunc(shutdownWait, func() {
		scr.Shutdown()
	})

	err = scr.Run()
	require.NoError(t, err)
	<-done
}

func testScannerInitialGetBlockHashError(t *testing.T, btcDB *bolt.DB) {
	// Test that scanner.Run() returns an error if the initial GetBlockHash
	// based upon scanner.cfg.InitialScanHeight fails
	scr, shutdown := setupScanner(t, btcDB)
	defer shutdown()

	// Empty the mock blockHashes map
	scr.btcClient.(*dummyBtcrpcclient).blockHashes = make(map[int64]string)

	err := scr.Run()
	require.Error(t, err)
	require.Equal(t, errNoBlockHash, err)
}

func TestScanner(t *testing.T) {
	btcDB := openDummyBtcDB(t)
	if !parallel {
		defer btcDB.Close()
	}

	t.Run("RunProcessDeposits", func(t *testing.T) {
		if parallel {
			t.Parallel()
		}
		testScannerRunProcessDeposits(t, btcDB)
	})

	t.Run("GetBlockCountErrorRetry", func(t *testing.T) {
		if parallel {
			t.Parallel()
		}
		testScannerGetBlockCountErrorRetry(t, btcDB)
	})

	t.Run("InitialGetBlockHashError", func(t *testing.T) {
		if parallel {
			t.Parallel()
		}
		testScannerInitialGetBlockHashError(t, btcDB)
	})

	t.Run("ProcessDepositError", func(t *testing.T) {
		if parallel {
			t.Parallel()
		}
		testScannerProcessDepositError(t, btcDB)
	})

	t.Run("ConfirmationsRequired", func(t *testing.T) {
		if parallel {
			t.Parallel()
		}
		testScannerConfirmationsRequired(t, btcDB)
	})

	t.Run("ScanBlockFailureRetry", func(t *testing.T) {
		if parallel {
			t.Parallel()
		}
		testScannerScanBlockFailureRetry(t, btcDB)
	})

	t.Run("LoadUnprocessedDeposits", func(t *testing.T) {
		if parallel {
			t.Parallel()
		}
		testScannerLoadUnprocessedDeposits(t, btcDB)
	})

	t.Run("DuplicateDepositScans", func(t *testing.T) {
		if parallel {
			t.Parallel()
		}
		testScannerDuplicateDepositScans(t, btcDB)
	})

	t.Run("BlockNextHashAppears", func(t *testing.T) {
		if parallel {
			t.Parallel()
		}
		testScannerBlockNextHashAppears(t, btcDB)
	})
}
