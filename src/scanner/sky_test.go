package scanner

import (
	"errors"
	"testing"
	"time"

	"github.com/boltdb/bolt"
	"github.com/stretchr/testify/require"

	"github.com/skycoin/teller/src/config"
	"github.com/skycoin/teller/src/util/dbutil"
	"github.com/skycoin/teller/src/util/testutil"

	"encoding/binary"

	"github.com/skycoin/skycoin/src/cipher/encoder"
	"github.com/skycoin/skycoin/src/coin"
	"github.com/skycoin/skycoin/src/visor"
)

var (
	dummySkyBlocksBktName = []byte("blocks")
	dummySkyTreeBktName   = []byte("block_tree")
	errNoSkyBlockHash     = errors.New("no block found for hash")
	errNoSkyBlockHeight   = errors.New("no block found for height")
)

type dummySkyrpcclient struct {
	db                           *bolt.DB
	blockHashes                  map[int64]string
	blockCount                   int64
	blockCountError              error
	blockVerboseTxError          error
	blockVerboseTxErrorCallCount int
	blockVerboseTxCallCount      int

	// used for testBtcScannerBlockNextHashAppears
	blockNextHeightMissingOnceAt uint64
	hasSetMissingHeight          bool
}

func openDummySkyDB(t *testing.T) *bolt.DB {
	// Blocks 0 through 180 are stored in this DB
	db, err := bolt.Open("./sky.db", 0600, &bolt.Options{ReadOnly: true})
	require.NoError(t, err)
	return db
}

func newDummySkyrpcclient(db *bolt.DB) *dummySkyrpcclient {
	return &dummySkyrpcclient{
		db:          db,
		blockHashes: make(map[int64]string),
	}
}

func (dsc *dummySkyrpcclient) GetBlockCount() (int64, error) {
	if dsc.blockCountError != nil {
		// blockCountError is only returned once
		err := dsc.blockCountError
		dsc.blockCountError = nil
		return 0, err
	}

	return dsc.blockCount, nil
}

func (dsc *dummySkyrpcclient) GetBlockVerboseTx(seq uint64) (*visor.ReadableBlock, error) {
	if seq > 0 && seq == dsc.blockNextHeightMissingOnceAt && !dsc.hasSetMissingHeight {
		dsc.hasSetMissingHeight = true
		return nil, errNoSkyBlockHash
	}

	dsc.blockVerboseTxCallCount++
	if dsc.blockVerboseTxCallCount == dsc.blockVerboseTxErrorCallCount {
		return nil, dsc.blockVerboseTxError
	}

	var block *visor.ReadableBlock
	if err := dsc.db.View(func(tx *bolt.Tx) error {
		var err error

		// tree bucket cursor
		c := tx.Bucket(dummySkyTreeBktName).Cursor()
		// search for the required key, need to first convert depth to []byte
		_, pairsBin := c.Seek(Itob(seq))
		if pairsBin == nil {
			return errNoSkyBlockHeight
		}
		pairs := []coin.HashPair{}
		// deserialize from binary to hashpair
		if err := encoder.DeserializeRaw(pairsBin, &pairs); err != nil {
			return err
		}
		// get hash from hashpair
		hash := visor.DefaultWalker(pairs)

		// block bucket cursor
		bc := tx.Bucket(dummySkyBlocksBktName).Cursor()
		// search for required hash
		_, bin := bc.Seek(hash[:])
		if bin == nil {
			return errNoSkyBlockHash
		}

		// deserialize from binary to block
		cblock := coin.Block{}
		if err := encoder.DeserializeRaw(bin, &cblock); err != nil {
			return err
		}

		// covert coin.Block to readableblock
		block, err = visor.NewReadableBlock(&cblock)
		return err
	}); err != nil {
		return nil, err
	}

	return block, nil
}

// Itob converts uint64 to bytes
func Itob(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return b
}

func (dsc *dummySkyrpcclient) Shutdown() {}

func setupSkyScannerWithDB(t *testing.T, skyDB *bolt.DB, db *bolt.DB) *SKYScanner {
	log, _ := testutil.NewLogger(t)

	rpc := newDummySkyrpcclient(skyDB)

	rpc.blockCount = 180

	store, err := NewStore(log, db)
	require.NoError(t, err)

	err = store.AddSupportedCoin(config.CoinTypeSKY)
	require.NoError(t, err)

	cfg := Config{
		ScanPeriod:            time.Millisecond * 10,
		DepositBufferSize:     2,
		InitialScanHeight:     0,
		ConfirmationsRequired: 0,
	}

	scr, err := NewSKYScanner(log, store, rpc, cfg)
	require.NoError(t, err)

	return scr

}

func setupSkyScannerWithNonExistInitHeight(t *testing.T, skyDB *bolt.DB, db *bolt.DB) *SKYScanner {
	log, _ := testutil.NewLogger(t)

	rpc := newDummySkyrpcclient(skyDB)

	rpc.blockCount = 180

	store, err := NewStore(log, db)
	require.NoError(t, err)
	err = store.AddSupportedCoin(config.CoinTypeSKY)
	require.NoError(t, err)

	cfg := Config{
		ScanPeriod:            time.Millisecond * 10,
		DepositBufferSize:     5,
		InitialScanHeight:     190,
		ConfirmationsRequired: 0,
	}
	scr, err := NewSKYScanner(log, store, rpc, cfg)
	require.NoError(t, err)

	return scr
}

func setupSkyScanner(t *testing.T, skyDB *bolt.DB) (*SKYScanner, func()) {
	db, shutdown := testutil.PrepareDB(t)

	scr := setupSkyScannerWithDB(t, skyDB, db)

	return scr, shutdown
}

func testSkyScannerRunProcessedLoop(t *testing.T, scr *SKYScanner, nDeposits int) {
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
		err := scr.base.GetStorer().(*Store).db.View(func(tx *bolt.Tx) error {
			for _, dv := range dvs {
				var d Deposit
				err := dbutil.GetBucketObject(tx, DepositBkt, dv.ID(), &d)
				require.NoError(t, err)
				if err != nil {
					return err
				}

				require.True(t, d.Processed)
				require.Equal(t, config.CoinTypeSKY, d.CoinType)
				require.NotEmpty(t, d.Address)
				if d.Value != 0 {
					require.NotEmpty(t, d.Value)
				}
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
	shutdownWait := scr.base.(*BaseScanner).Cfg.ScanPeriod * time.Duration(nDeposits*2)
	if shutdownWait < *minShutdownWait {
		shutdownWait = *minShutdownWait
	}

	time.AfterFunc(shutdownWait, func() {
		scr.Shutdown()
	})

	err := scr.Run()
	require.NoError(t, err)
	<-done
}

func testSkyScannerRun(t *testing.T, scr *SKYScanner) {
	nDeposits := 0

	// This address has:
	// 1 deposit, in block 176
	err := scr.AddScanAddress("v4qF7Ceq276tZpTS3HKsZbDguMAcAGAG1q", config.CoinTypeSKY)
	require.NoError(t, err)
	nDeposits = nDeposits + 1

	// This address has:
	// 1 deposit in block 116
	// 1 deposit in block 117
	err = scr.AddScanAddress("8MQsjc5HYbSjPTZikFZYeHHDtLungBEHYS", config.CoinTypeSKY)
	require.NoError(t, err)
	nDeposits = nDeposits + 2

	// Make sure that the deposit buffer size is less than the number of deposits,
	// to test what happens when the buffer is full
	require.True(t, scr.base.(*BaseScanner).Cfg.DepositBufferSize < nDeposits)

	testSkyScannerRunProcessedLoop(t, scr, nDeposits)
}

func testSkyScannerRunProcessDeposits(t *testing.T, skyDB *bolt.DB) {
	// Tests that the scanner will scan multiple blocks sequentially, finding
	// all relevant deposits and adding them to the depositC channel.
	// All deposits on the depositC channel will be successfully processed
	// by the channel reader, and the scanner will mark these deposits as
	// "processed".
	scr, shutdown := setupSkyScanner(t, skyDB)
	defer shutdown()

	testSkyScannerRun(t, scr)
}

func testSkyScannerInitialGetBlockHashError(t *testing.T, skyDB *bolt.DB) {
	// Test that scanner.Run() returns an error if the initial GetBlockHash
	// based upon scanner.base.Cfg.InitialScanHeight fails
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	scr := setupSkyScannerWithNonExistInitHeight(t, skyDB, db)

	err := scr.Run()
	require.Error(t, err)
	require.Equal(t, errNoSkyBlockHeight, err)
}

func testSkyScannerGetBlockCountErrorRetry(t *testing.T, skyDB *bolt.DB) {
	// Test that if the scanner scan loop encounters an error when calling
	// GetBlockCount(), the loop continues to work fine
	// This test is that same as testSkyScannerRunProcessDeposits,
	// except that the dummySkyrpcclient is configured to return an error
	// from GetBlockCount() one time
	scr, shutdown := setupSkyScanner(t, skyDB)
	defer shutdown()

	scr.skyClient.(*dummySkyrpcclient).blockCountError = errors.New("block count error")

	testSkyScannerRun(t, scr)
}

func testSkyScannerProcessDepositError(t *testing.T, skyDB *bolt.DB) {
	// Test that when processDeposit() fails, the deposit is NOT marked as processed
	scr, shutdown := setupSkyScanner(t, skyDB)
	defer shutdown()

	var nDeposits int64

	// This address has deposits in: Block 52, 54, 59, 108, 134, 137, 141
	err := scr.AddScanAddress("2J3rWX7pciQwmvcATSnxEeCHRs1mSkWmt4L", config.CoinTypeSKY)
	require.NoError(t, err)
	nDeposits = nDeposits + 7

	// Make sure that the deposit buffer size is less than the number of deposits,
	// to test what happens when the buffer is full
	require.True(t, int64(scr.base.(*BaseScanner).Cfg.DepositBufferSize) < nDeposits)

	done := make(chan struct{})
	go func() {
		defer close(done)
		var dvs []DepositNote
		for dv := range scr.GetDeposit() {
			dvs = append(dvs, dv)
			dv.ErrC <- errors.New("failed to process deposit")
		}

		require.Equal(t, nDeposits, int64(len(dvs)))

		// check all deposits, none should be marked as "Processed"
		err := scr.base.GetStorer().(*Store).db.View(func(tx *bolt.Tx) error {
			for _, dv := range dvs {
				var d Deposit
				err := dbutil.GetBucketObject(tx, DepositBkt, dv.ID(), &d)
				require.NoError(t, err)
				if err != nil {
					return err
				}

				//require.False(t, d.Processed)
				require.Equal(t, config.CoinTypeSKY, d.CoinType)
				require.Equal(t, "2J3rWX7pciQwmvcATSnxEeCHRs1mSkWmt4L", d.Address)
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
	shutdownWait := time.Duration(int64(scr.base.(*BaseScanner).Cfg.ScanPeriod) * nDeposits * 2)
	if shutdownWait < *minShutdownWait {
		shutdownWait = *minShutdownWait
	}

	time.AfterFunc(shutdownWait, func() {
		scr.Shutdown()
	})

	err = scr.Run()
	require.NoError(t, err)
	<-done
}

func testSkyScannerScanBlockFailureRetry(t *testing.T, skyDB *bolt.DB) {
	// Test that when scanBlock() fails, it logs "Scan block failed"
	// and retries scan of the same block after ScanPeriod elapses.
	scr, shutdown := setupSkyScanner(t, skyDB)
	defer shutdown()

	// Return an error on the 2nd call to GetBlockVerboseTx
	scr.skyClient.(*dummySkyrpcclient).blockVerboseTxError = errors.New("get block verbose tx error")
	scr.skyClient.(*dummySkyrpcclient).blockVerboseTxErrorCallCount = 2

	testSkyScannerRun(t, scr)
}

func testSkyScannerLoadUnprocessedDeposits(t *testing.T, skyDB *bolt.DB) {
	// Test that pending unprocessed deposits from the db are loaded when
	// then scanner starts.
	scr, shutdown := setupSkyScanner(t, skyDB)
	defer shutdown()

	// NOTE: This data is fake, but the addresses and Txid are valid
	unprocessedDeposits := []Deposit{
		{
			CoinType:  config.CoinTypeSKY,
			Address:   "2J3rWX7pciQwmvcATSnxEeCHRs1mSkWmt4L",
			Value:     1e8,
			Height:    141,
			Tx:        "16f8b9369f76ef6a0c1ecf82e1c18d5bc8ae5ef8b01b6530096cb1ff70bbd3fd",
			N:         1,
			Processed: false,
		},
		{
			CoinType:  config.CoinTypeSKY,
			Address:   "VD98Qt2f2UeUbUKcCJEaKxqEewExgCyiVh",
			Value:     10e8,
			Height:    115,
			Tx:        "bb700553c3e1a32346912ab311fa38793d929f311daeee0b167fa81c1369717e",
			N:         1,
			Processed: false,
		},
	}

	processedDeposit := Deposit{
		CoinType:  config.CoinTypeSKY,
		Address:   "2iJPqYVuQvFoG1pim4bjoyxWK8uwGmznWaV",
		Value:     100e8,
		Height:    163,
		Tx:        "ec79854fade530d84099d5619864a8e1e8ec9d27a086917a239500cada43c6e8",
		N:         1,
		Processed: true,
	}

	err := scr.base.GetStorer().(*Store).db.Update(func(tx *bolt.Tx) error {
		for _, d := range unprocessedDeposits {
			if err := scr.base.GetStorer().(*Store).pushDepositTx(tx, d); err != nil {
				require.NoError(t, err)
				return err
			}
		}

		// Add a processed deposit to make sure that processed deposits are filtered
		return scr.base.GetStorer().(*Store).pushDepositTx(tx, processedDeposit)
	})
	require.NoError(t, err)

	// Don't add any watch addresses,
	// only process the unprocessed deposits from the backlog
	testSkyScannerRunProcessedLoop(t, scr, len(unprocessedDeposits))
}

func testSkyScannerDuplicateDepositScans(t *testing.T, skyDB *bolt.DB) {
	// Test that rescanning the same blocks doesn't send extra deposits
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	var nDeposits int

	scr := setupSkyScannerWithDB(t, skyDB, db)

	// This address has:
	// 1 deposit in block 116
	// 1 deposit in block 117
	err := scr.AddScanAddress("8MQsjc5HYbSjPTZikFZYeHHDtLungBEHYS", config.CoinTypeSKY)
	require.NoError(t, err)
	nDeposits = nDeposits + 2

	testSkyScannerRunProcessedLoop(t, scr, nDeposits)

	// Scanning again will have no new deposits
	scr = setupSkyScannerWithDB(t, skyDB, db)
	testSkyScannerRunProcessedLoop(t, scr, 0)
}

func testSkyScannerBlockNextHashAppears(t *testing.T, skyDB *bolt.DB) {
	scr, shutdown := setupSkyScanner(t, skyDB)
	defer shutdown()

	scr.skyClient.(*dummySkyrpcclient).blockNextHeightMissingOnceAt = 178

	testSkyScannerRun(t, scr)
}

func TestSkyScanner(t *testing.T) {
	skyDB := openDummySkyDB(t)
	defer testutil.CheckError(t, skyDB.Close)

	t.Run("group", func(t *testing.T) {
		t.Run("RunProcessDeposits", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testSkyScannerRunProcessDeposits(t, skyDB)
		})

		t.Run("GetBlockCountErrorRetry", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testSkyScannerGetBlockCountErrorRetry(t, skyDB)
		})

		t.Run("InitialGetBlockHashError", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testSkyScannerInitialGetBlockHashError(t, skyDB)
		})

		t.Run("ProcessDepositError", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testSkyScannerProcessDepositError(t, skyDB)
		})

		t.Run("ScanBlockFailureRetry", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testSkyScannerScanBlockFailureRetry(t, skyDB)
		})

		t.Run("LoadUnprocessedDeposits", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testSkyScannerLoadUnprocessedDeposits(t, skyDB)
		})

		t.Run("DuplicateDepositScans", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testSkyScannerDuplicateDepositScans(t, skyDB)
		})

		t.Run("BlockNextHashAppears", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testSkyScannerBlockNextHashAppears(t, skyDB)
		})
	})

}
