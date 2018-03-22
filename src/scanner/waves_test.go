package scanner

import (
	"errors"
	"testing"
	"time"

	"github.com/boltdb/bolt"
	"github.com/stretchr/testify/require"

	"github.com/MDLlife/teller/src/util/dbutil"
	"github.com/MDLlife/teller/src/util/testutil"

	"log"

	"github.com/modeneis/waves-go-client/client"
	"github.com/modeneis/waves-go-client/model"
)

var (
	errNoWavesBlockHash = errors.New("Block 2 not found")
	//testWebRPCAddr    = "127.0.0.1:8081"
	//txHeight          = uint64(103)
	//txConfirmed       = true
)

type wavesFakeGateway struct {
	transactions         map[string]string
	injectRawTxMap       map[string]bool // key: transaction hash, value indicates whether the injectTransaction should return error.
	injectedTransactions map[string]string
	//addrRecvUxOuts       []*historydb.UxOut
	//addrSpentUxOUts      []*historydb.UxOut
}

type dummyWavesrpcclient struct {
	db                           *bolt.DB
	blockHashes                  map[int64]string
	blockCount                   int64
	blockCountError              error
	blockVerboseTxError          error
	blockVerboseTxErrorCallCount int
	//blockVerboseTxCallCount      int

	// used for testWavesScannerBlockNextHashAppears
	blockNextHashMissingOnceAt int64
	//hasSetMissingHash          bool

	//log          logrus.FieldLogger
	Base       CommonScanner
	walletFile string
	changeAddr string
}

func openDummyWavesDB(t *testing.T) *bolt.DB {
	// Blocks 2325205 through 2325214 are stored in this DB
	db, err := bolt.Open("./waves.db", 0600, nil)
	require.NoError(t, err)
	return db
}

func setupWavesScannerWithDB(t *testing.T, wavesDB *bolt.DB, db *bolt.DB) *WAVESScanner {
	log, _ := testutil.NewLogger(t)

	store, err := NewStore(log, db)
	require.NoError(t, err)
	err = store.AddSupportedCoin(CoinTypeWAVES)
	require.NoError(t, err)

	// Block 2 doesn't exist in db
	cfg := Config{
		ScanPeriod:            time.Millisecond * 5,
		DepositBufferSize:     5,
		InitialScanHeight:     924610,
		ConfirmationsRequired: 0,
	}
	scr, err := NewWavescoinScanner(log, store, new(dummyWavesrpcclient), cfg)
	require.NoError(t, err)

	return scr
}

func setupWavesScanner(t *testing.T, wavesDB *bolt.DB) (*WAVESScanner, func()) {
	db, shutdown := testutil.PrepareDB(t)

	scr := setupWavesScannerWithDB(t, wavesDB, db)

	return scr, shutdown
}

func testWavesScannerRunProcessedLoop(t *testing.T, scr *WAVESScanner, nDeposits int) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		var dvs []DepositNote
		for dv := range scr.GetDeposit() {
			dvs = append(dvs, dv)
			dv.ErrC <- nil
		}

		require.True(t, len(dvs) >= nDeposits)

		// check all deposits
		err := scr.Base.GetStorer().(*Store).db.View(func(tx *bolt.Tx) error {
			for _, dv := range dvs {
				var d Deposit
				err := dbutil.GetBucketObject(tx, DepositBkt, dv.ID(), &d)
				require.NoError(t, err)
				if err != nil {
					return err
				}

				require.True(t, d.Processed)
				require.Equal(t, CoinTypeWAVES, d.CoinType)
				require.NotEmpty(t, d.Address)
				if d.Value != 0 { // value(0x87b127ee022abcf9881b9bad6bb6aac25229dff0) = 0
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
	shutdownWait := scr.Base.(*BaseScanner).Cfg.ScanPeriod * time.Duration(nDeposits*2)
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

func testWavesScannerRun(t *testing.T, scr *WAVESScanner) {
	nDeposits := 0

	// This address has 0 deposits
	err := scr.AddScanAddress("3PFTGLDvE7rQfWtgSzBt7NS4NXXMQ1gUufs", CoinTypeWAVES)
	require.NoError(t, err)
	nDeposits = nDeposits + 1

	// This address has:
	// 1 deposit, in block 2325212
	err = scr.AddScanAddress("3P9dUze9nHRdfoKhFrZYKdsSpwW9JoE6Mzf", CoinTypeWAVES)
	require.NoError(t, err)
	nDeposits = nDeposits + 1

	// Make sure that the deposit buffer size is less than the number of deposits,
	// to test what happens when the buffer is full
	scr.Base.(*BaseScanner).Cfg.DepositBufferSize = 1
	log.Println("DepositBufferSize, ", scr.Base.(*BaseScanner).Cfg.DepositBufferSize)
	require.True(t, scr.Base.(*BaseScanner).Cfg.DepositBufferSize < nDeposits)

	testWavesScannerRunProcessedLoop(t, scr, nDeposits)
}

func testWavesScannerRunProcessDeposits(t *testing.T, wavesDB *bolt.DB) {
	// Tests that the scanner will scan multiple blocks sequentially, finding
	// all relevant deposits and adding them to the depositC channel.
	// All deposits on the depositC channel will be successfully processed
	// by the channel reader, and the scanner will mark these deposits as
	// "processed".
	scr, shutdown := setupWavesScanner(t, wavesDB)
	defer shutdown()

	testWavesScannerRun(t, scr)
}

func testWavesScannerGetBlockCountErrorRetry(t *testing.T, wavesDB *bolt.DB) {
	// Test that if the scanner scan loop encounters an error when calling
	// GetBlockCount(), the loop continues to work fine
	// This test is that same as testWavesScannerRunProcessDeposits,
	// except that the dummyWavesrpcclient is configured to return an error
	// from GetBlockCount() one time
	scr, shutdown := setupWavesScanner(t, wavesDB)
	defer shutdown()

	scr.wavesRPCClient.(*dummyWavesrpcclient).blockCountError = errors.New("block count error")

	testWavesScannerRun(t, scr)
}

func testWavesScannerConfirmationsRequired(t *testing.T, wavesDB *bolt.DB) {
	// Test that the scanner uses cfg.ConfirmationsRequired correctly
	scr, shutdown := setupWavesScanner(t, wavesDB)
	defer shutdown()

	// Scanning starts at block 2325212, set the blockCount height to 1
	// confirmations higher, so that only block 2325212 is processed.
	scr.Base.(*BaseScanner).Cfg.DepositBufferSize = 6
	scr.Base.(*BaseScanner).Cfg.ConfirmationsRequired = 0
	scr.wavesRPCClient.(*dummyWavesrpcclient).blockCount = 1

	// Add scan addresses for blocks 2325205-2325214, but only expect to scan
	// deposits from block 2325205-2325212, since 2325213 and 2325214 don't have enough
	// confirmations
	nDeposits := 0

	// This address has:
	// 2 deposits in block 2325212
	// Only blocks 2325212  are processed, because blockCount is set
	// to 2325214 and the confirmations required is set to 1
	err := scr.AddScanAddress("3P9dUze9nHRdfoKhFrZYKdsSpwW9JoE6Mzf", CoinTypeWAVES)
	require.NoError(t, err)
	nDeposits = nDeposits + 1

	// has't enough deposit
	require.True(t, scr.Base.(*BaseScanner).Cfg.DepositBufferSize > nDeposits)

	testWavesScannerRunProcessedLoop(t, scr, nDeposits)
}

func testWavesScannerScanBlockFailureRetry(t *testing.T, wavesDB *bolt.DB) {
	// Test that when scanBlock() fails, it logs "Scan block failed"
	// and retries scan of the same block after ScanPeriod elapses.
	scr, shutdown := setupWavesScanner(t, wavesDB)
	defer shutdown()

	// Return an error on the 2nd call to GetBlockVerboseTx
	scr.wavesRPCClient.(*dummyWavesrpcclient).blockVerboseTxError = errors.New("get block verbose tx error")
	scr.wavesRPCClient.(*dummyWavesrpcclient).blockVerboseTxErrorCallCount = 2

	testWavesScannerRun(t, scr)
}

func testWavesScannerBlockNextHashAppears(t *testing.T, wavesDB *bolt.DB) {
	// Test that when a block has no NextHash, the scanner waits until it has
	// one, then resumes normally
	scr, shutdown := setupWavesScanner(t, wavesDB)
	defer shutdown()

	// The block at height 2325208 will lack a NextHash one time
	// The scanner will continue and process everything normally
	scr.wavesRPCClient.(*dummyWavesrpcclient).blockNextHashMissingOnceAt = 2

	testWavesScannerRun(t, scr)
}

func testWavesScannerDuplicateDepositScans(t *testing.T, wavesDB *bolt.DB) {
	// Test that rescanning the same blocks doesn't send extra deposits
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	nDeposits := 0

	// This address has:
	// 2 deposit, in block 2325212
	scr := setupWavesScannerWithDB(t, wavesDB, db)
	err := scr.AddScanAddress("3PRDjxHwETEhYXM3tjVMU4oYhfj3dqT6Vuw", CoinTypeWAVES)
	require.NoError(t, err)
	nDeposits = nDeposits + 2

	testWavesScannerRunProcessedLoop(t, scr, nDeposits)

	// Scanning again will have no new deposits
	scr = setupWavesScannerWithDB(t, wavesDB, db)
	testWavesScannerRunProcessedLoop(t, scr, 0)
}

func testWavesScannerLoadUnprocessedDeposits(t *testing.T, wavesDB *bolt.DB) {
	// Test that pending unprocessed deposits from the db are loaded when
	// then scanner starts.
	scr, shutdown := setupWavesScanner(t, wavesDB)
	defer shutdown()

	// NOTE: This data is fake, but the addresses and Txid are valid
	unprocessedDeposits := []Deposit{
		{
			CoinType:  CoinTypeWAVES,
			Address:   "0x196736a260c6e7c86c88a73e2ffec400c9caef71",
			Value:     1e8,
			Height:    2325212,
			Tx:        "0xc724f4aae6f89e6296aec22c6795e7423b6776e2ee3c5f942cf3817a9ded0c32",
			N:         1,
			Processed: false,
		},
		{
			CoinType:  CoinTypeWAVES,
			Address:   "0x2a5ee9b4307a0030982ed00ca7e904a20fc53a12",
			Value:     10e8,
			Height:    2325212,
			Tx:        "0xca8d662c6cf2dcd0e8c9075b58bfbfa7ee4769e5efd6f45e490309d58074913e",
			N:         1,
			Processed: false,
		},
	}

	processedDeposit := Deposit{
		CoinType:  CoinTypeWAVES,
		Address:   "0x87b127ee022abcf9881b9bad6bb6aac25229dff0",
		Value:     100e8,
		Height:    2325212,
		Tx:        "0x01d15c4d79953e2c647ce668045e8d98369ff958b2b021fbdf9e39bceab3add9",
		N:         1,
		Processed: true,
	}

	err := scr.Base.GetStorer().(*Store).db.Update(func(tx *bolt.Tx) error {
		for _, d := range unprocessedDeposits {
			if err := scr.Base.GetStorer().(*Store).pushDepositTx(tx, d); err != nil {
				require.NoError(t, err)
				return err
			}
		}

		// Add a processed deposit to make sure that processed deposits are filtered
		return scr.Base.GetStorer().(*Store).pushDepositTx(tx, processedDeposit)
	})
	require.NoError(t, err)

	// Don't add any watch addresses,
	// only process the unprocessed deposits from the backlog
	testWavesScannerRunProcessedLoop(t, scr, len(unprocessedDeposits))
}

func testWavesScannerProcessDepositError(t *testing.T, wavesDB *bolt.DB) {
	// Test that when processDeposit() fails, the deposit is NOT marked as processed
	scr, shutdown := setupWavesScanner(t, wavesDB)
	defer shutdown()

	nDeposits := 0

	scr.Base.(*BaseScanner).Cfg.DepositBufferSize = 2
	scr.Base.(*BaseScanner).Cfg.ScanPeriod = 5
	// This address has:
	// 9 deposits in block 2325213
	err := scr.AddScanAddress("3PRDjxHwETEhYXM3tjVMU4oYhfj3dqT6Vuw", CoinTypeWAVES)
	require.NoError(t, err)
	nDeposits = nDeposits + 2

	err = scr.AddScanAddress("3P9dUze9nHRdfoKhFrZYKdsSpwW9JoE6Mzf", CoinTypeWAVES)
	require.NoError(t, err)
	nDeposits = nDeposits + 1

	// Make sure that the deposit buffer size is less than the number of deposits,
	// to test what happens when the buffer is full
	require.True(t, scr.Base.(*BaseScanner).Cfg.DepositBufferSize <= nDeposits)

	done := make(chan struct{})
	go func() {
		defer close(done)
		var dvs []DepositNote
		for dv := range scr.GetDeposit() {
			dvs = append(dvs, dv)
			dv.ErrC <- errors.New("failed to process deposit")
		}

		require.True(t, len(dvs) >= nDeposits)

		// check all deposits, none should be marked as "Processed"
		err := scr.Base.GetStorer().(*Store).db.View(func(tx *bolt.Tx) error {
			for _, dv := range dvs {
				var d Deposit
				err := dbutil.GetBucketObject(tx, DepositBkt, dv.ID(), &d)
				require.NoError(t, err)
				if err != nil {
					return err
				}

				require.False(t, d.Processed)
				require.Equal(t, CoinTypeWAVES, d.CoinType)
				require.Contains(t, []string{"3PRDjxHwETEhYXM3tjVMU4oYhfj3dqT6Vuw", "3P9dUze9nHRdfoKhFrZYKdsSpwW9JoE6Mzf"}, d.Address)

				if d.Value != 0 { //value(0x87b127ee022abcf9881b9bad6bb6aac25229dff0) = 0
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
	shutdownWait := scr.Base.(*BaseScanner).Cfg.ScanPeriod * time.Duration(nDeposits*2)
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

func testWavesScannerInitialGetBlockHashError(t *testing.T, wavesDB *bolt.DB) {
	// Test that scanner.Run() returns an error if the initial GetBlockHash
	// based upon scanner.Base.Cfg.InitialScanHeight fails
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	scr := setupWavesScannerWithDB(t, wavesDB, db)

	err := scr.Run()
	require.Error(t, err)
	require.Equal(t, errNoWavesBlockHash, err)
}

func TestWavesScanner(t *testing.T) {
	wavesDB := openDummyWavesDB(t)
	defer testutil.CheckError(t, wavesDB.Close)
	t.Run("group", func(t *testing.T) {

		t.Run("RunProcessDeposits", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testWavesScannerRunProcessDeposits(t, wavesDB)
		})

		t.Run("GetBlockCountErrorRetry", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testWavesScannerGetBlockCountErrorRetry(t, wavesDB)
		})

		//t.Run("InitialGetBlockHashError", func(t *testing.T) {
		//	if parallel {
		//		t.Parallel()
		//	}
		//	testWavesScannerInitialGetBlockHashError(t, wavesDB)
		//})

		t.Run("ProcessDepositError", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testWavesScannerProcessDepositError(t, wavesDB)
		})

		t.Run("ConfirmationsRequired", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testWavesScannerConfirmationsRequired(t, wavesDB)
		})

		t.Run("ScanBlockFailureRetry", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testWavesScannerScanBlockFailureRetry(t, wavesDB)
		})

		t.Run("LoadUnprocessedDeposits", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testWavesScannerLoadUnprocessedDeposits(t, wavesDB)
		})

		t.Run("DuplicateDepositScans", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testWavesScannerDuplicateDepositScans(t, wavesDB)
		})

		t.Run("BlockNextHashAppears", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testWavesScannerBlockNextHashAppears(t, wavesDB)
		})
	})
}

// GetTransaction returns transaction by txid
func (c *dummyWavesrpcclient) GetTransaction(txid string) (*model.Transactions, error) {
	transaction, _, err := client.NewTransactionsService().GetTransactionsInfoID(txid)
	return transaction, err
}

func (c *dummyWavesrpcclient) GetBlocks(start, end int64) (*[]model.Blocks, error) {
	blocks, _, err := client.NewBlocksService().GetBlocksSeqFromTo(start, end)
	return blocks, err
}

func (c *dummyWavesrpcclient) GetBlocksBySeq(seq int64) (*model.Blocks, error) {
	block, _, err := client.NewBlocksService().GetBlocksAtHeight(seq)
	return block, err
}

func (c *dummyWavesrpcclient) GetLastBlocks() (*model.Blocks, error) {
	blocks, _, err := client.NewBlocksService().GetBlocksLast()
	return blocks, err
}

func (c *dummyWavesrpcclient) Shutdown() {
}
