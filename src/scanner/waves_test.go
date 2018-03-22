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
	"fmt"
	"encoding/json"
)

var (
	errNoWavesBlockHash = errors.New("Block 666 not found")
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
	forceErr                     bool
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

func setupWavesScannerWithNonExistInitHeight(t *testing.T, db *bolt.DB) *WAVESScanner {
	log, _ := testutil.NewLogger(t)

	rpc := new(dummyWavesrpcclient)

	store, err := NewStore(log, db)
	require.NoError(t, err)
	err = store.AddSupportedCoin(CoinTypeWAVES)
	require.NoError(t, err)

	// Block 2 doesn't exist in db
	cfg := Config{
		ScanPeriod:            time.Millisecond * 10,
		DepositBufferSize:     5,
		InitialScanHeight:     666,
		ConfirmationsRequired: 0,
	}
	scr, err := NewWavescoinScanner(log, store, rpc, cfg)
	require.NoError(t, err)

	return scr
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

		require.Equal(t, nDeposits, len(dvs))

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

	// This address has 1 deposits
	err := scr.AddScanAddress("3PFTGLDvE7rQfWtgSzBt7NS4NXXMQ1gUufs", CoinTypeWAVES)
	require.NoError(t, err)
	nDeposits = nDeposits + 1

	// This address has:
	// 1 deposit
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
	// 1 deposit
	scr := setupWavesScannerWithDB(t, wavesDB, db)
	err := scr.AddScanAddress("3PRDjxHwETEhYXM3tjVMU4oYhfj3dqT6Vuw", CoinTypeWAVES)
	require.NoError(t, err)
	nDeposits = nDeposits + 1

	// This address has:
	// 1 deposit, in block 2325212
	err = scr.AddScanAddress("3P9dUze9nHRdfoKhFrZYKdsSpwW9JoE6Mzf", CoinTypeWAVES)
	require.NoError(t, err)
	nDeposits = nDeposits + 1

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
	nDeposits = nDeposits + 1

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

		require.Equal(t, nDeposits, len(dvs))

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

func testWavesScannerInitialGetBlockHashError(t *testing.T, db *bolt.DB) {
	// Test that scanner.Run() returns an error if the initial GetBlockHash
	// based upon scanner.Base.Cfg.InitialScanHeight fails

	scr := setupWavesScannerWithNonExistInitHeight(t, db)

	// Empty the mock blockHashes map
	scr.wavesRPCClient.(*dummyWavesrpcclient).blockHashes = make(map[int64]string)
	scr.wavesRPCClient.(*dummyWavesrpcclient).forceErr = true

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

		t.Run("InitialGetBlockHashError", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testWavesScannerInitialGetBlockHashError(t, wavesDB)
		})

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
	if c.forceErr == true {
		return nil, fmt.Errorf("Block %d not found", seq)
	}
	blocks := decodeBlockWaves(blockStringWaves)
	return blocks, nil
}

func (c *dummyWavesrpcclient) GetLastBlocks() (*model.Blocks, error) {
	blocks := decodeBlockWaves(blockStringWaves)
	return blocks, nil
}

func decodeBlockWaves(str string) *model.Blocks {
	var blocks *model.Blocks
	if err := json.Unmarshal([]byte(str), &blocks); err != nil {
		panic(err)
	}
	return blocks
}

func (c *dummyWavesrpcclient) Shutdown() {
}

var blockStringWaves = `{
  "version": 3,
  "timestamp": 1521485631005,
  "reference": "3wt76w9MMUfiqfqvBJFRPApu6neGfGUjTyaR8WJUrnk5fiQKSPvhDsB6DK7CBvmCtyfv79h2x3LAoWwfgfpLfAw9",
  "nxt-consensus": {
    "base-target": 50,
    "generation-signature": "AhHzvMzifbubtGQR8xi1kdyLAzBmRW8jukn5oetWqvcE"
  },
  "features": [
    3
  ],
  "generator": "3PNMvAqJWYPkwf8fhz46rZiLEWpTmuhD3Uh",
  "signature": "4FzYYpSMXxW9fzMWEnMBh5q9X7joRC5rCxz41CZ7p2J7Q6V8KVMArQStxqeLCwzJRmf8neFT8KDzG3R8wjVjd96D",
  "blocksize": 4678,
  "transactionCount": 15,
  "fee": 2500000,
  "transactions": [
    {
      "type": 7,
      "id": "FpCV3gEKqPjtkY6Pu7BthZUeSWA1nTpEFetitm8Zx3Z6",
      "sender": "3PJaDyprvekvPXPuAtxrapacuDJopgJRaU3",
      "senderPublicKey": "7kPFrHDiGw1rCm7LPszuECwWYL3dMf6iMifLRDJQZMzy",
      "fee": 300000,
      "timestamp": 1521485623488,
      "signature": "5R7hzVME7XnaqfiqSoJWwSoQ2PPeuYNcRwyduVU4XwCMQadfnJgcjUGLHnaLP4L7vU25XaLfGfiZ4gwbMRudy3Ui",
      "order1": {
        "id": "5eaYnNAA8cuXKjJsfzcvH6bEGsSzDuWVuxnq61kbhaFs",
        "senderPublicKey": "9gQY94enLzH3TW3WowJzoQDSU3kJjHp6eUgchUC5Z2r4",
        "matcherPublicKey": "7kPFrHDiGw1rCm7LPszuECwWYL3dMf6iMifLRDJQZMzy",
        "assetPair": {
          "amountAsset": "B1u2TBpTYHWCuMuKLnbQfLvdLJ3zjgPiy3iMS2TSYugZ",
          "priceAsset": null
        },
        "orderType": "buy",
        "price": 12970000000,
        "amount": 33000000,
        "timestamp": 1521485622000,
        "expiration": 1521572022000,
        "matcherFee": 300000,
        "signature": "2HPAhRuSddFdcpz5zWnB65FRnAemoyhrHwgb1BAbH7TwwYMGbtp8cQLS3LDrLz81RtiZUoAxqV528DFt9xacB7qu"
      },
      "order2": {
        "id": "7mZtsUtTP1uGseNz6JJDgCqNFFNswHj1H3bnVVYn3C5i",
        "senderPublicKey": "9gQY94enLzH3TW3WowJzoQDSU3kJjHp6eUgchUC5Z2r4",
        "matcherPublicKey": "7kPFrHDiGw1rCm7LPszuECwWYL3dMf6iMifLRDJQZMzy",
        "assetPair": {
          "amountAsset": "B1u2TBpTYHWCuMuKLnbQfLvdLJ3zjgPiy3iMS2TSYugZ",
          "priceAsset": null
        },
        "orderType": "sell",
        "price": 12970000000,
        "amount": 33000000,
        "timestamp": 1521485623000,
        "expiration": 1521572023000,
        "matcherFee": 300000,
        "signature": "3qHBJcobjZgXCQy7VCQ5n53JEbcqgjN8KCycTqbf9KZcwYLpt8ErN8o2bN6H5FazETViaUKMCJza34xNi7ggPNu6"
      },
      "price": 12970000000,
      "amount": 33000000,
      "buyMatcherFee": 300000,
      "sellMatcherFee": 300000
    },
    {
      "type": 7,
      "id": "58esX9Kg1qgworrYGHvqF2pnVVfTvY915gds9w2Ng1AA",
      "sender": "3PJaDyprvekvPXPuAtxrapacuDJopgJRaU3",
      "senderPublicKey": "7kPFrHDiGw1rCm7LPszuECwWYL3dMf6iMifLRDJQZMzy",
      "fee": 300000,
      "timestamp": 1521485626132,
      "signature": "5R8naBLVqYPEnMauth2BPq79AnZfE1jLTrm6EGBSEXLr5rBHGbfSByKHiZyyz1wCWfwvrZ2bEyY3NrnLnR954mRo",
      "order1": {
        "id": "Aev7U8xRUf84LmNeGJsVpYqK8G2TanfhTvdq2qzsiYqm",
        "senderPublicKey": "ED8wfYB7DT3fcQRqFBUt6nYE1fEN6hwvCrtfrCW1GEuX",
        "matcherPublicKey": "7kPFrHDiGw1rCm7LPszuECwWYL3dMf6iMifLRDJQZMzy",
        "assetPair": {
          "amountAsset": "4XUN5uu8EaQSaKiC9jbEK3DN5peAoTDSnHZGyRWwyniC",
          "priceAsset": null
        },
        "orderType": "buy",
        "price": 138999910000000,
        "amount": 1,
        "timestamp": 1521484998339,
        "expiration": 1523990598339,
        "matcherFee": 300000,
        "signature": "3v3xE5NowyEohm7SwXtCvNhVD5T2ksTPnRZnjURGdSJ5rEEkrEq5zCyw79LAuW48WGJWhRwCjMREM6auzxdkxh2i"
      },
      "order2": {
        "id": "B7zCZQ8WExxXeebdWsyuRXtbB1daeNR2PLmC7dT4Xeo3",
        "senderPublicKey": "8Us7YHE3xdvcQMQBGpuZL5ij3aNr6nQ3zrAFgZLTy2FA",
        "matcherPublicKey": "7kPFrHDiGw1rCm7LPszuECwWYL3dMf6iMifLRDJQZMzy",
        "assetPair": {
          "amountAsset": "4XUN5uu8EaQSaKiC9jbEK3DN5peAoTDSnHZGyRWwyniC",
          "priceAsset": null
        },
        "orderType": "sell",
        "price": 138999910000000,
        "amount": 461,
        "timestamp": 1521485625967,
        "expiration": 1523213625967,
        "matcherFee": 300000,
        "signature": "5d4UFXqKsjvdWGEdYFoQLkv2HSRE6W2fKMGsLHxfMoU4iJjknMCwaiajX6E5Q1mMb9LgtzrX11Dta4Gv6P5jKAbc"
      },
      "price": 138999910000000,
      "amount": 1,
      "buyMatcherFee": 300000,
      "sellMatcherFee": 650
    },
    {
      "type": 4,
      "id": "Ee9Ji5PropqoDW2FzyyQsVEobmTHRRbtPTfEzufjkrsi",
      "sender": "3P2u3LjeBXbVGfUVS6JnxknJBgXtAJRLttF",
      "senderPublicKey": "7Dq3SwFoojdvxkbvyUFH5WNDViyv8FD9FtCBwFuagaxE",
      "fee": 100000,
      "timestamp": 1521485621237,
      "signature": "2hgKLeRwaxxCjwPBuGsY483pkyCSfDwJ3ZQS2PUssyP8DwogMTe4WWmcouixPkuYfXNshv1StztrWWiB2cxmSwmu",
      "recipient": "3P9ZK7jG2shcoWvyjhbGrL8QoS5yBZAMzMn",
      "assetId": null,
      "amount": 199900000,
      "feeAsset": null,
      "attachment": ""
    },
    {
      "type": 4,
      "id": "KSa8ybJXdQRwqxUNV9UnybZjG7bstS2ig35UErr3oqH",
      "sender": "3PDMYU97isdBpXHkjepokeHLzgqcwBnTJED",
      "senderPublicKey": "7iW7H1KTMr7q6sDCZeavFxHjExyKySCo5LvvQcugxxfn",
      "fee": 100000,
      "timestamp": 1521485624643,
      "signature": "oZtVc8EV3uyNanJS4Fu4K8NMJjiYDsfZMZKQFwLXz2Pqjq67afRniBNVm7Nxwg3LmYJyx2ey8MoWknczr3NDzQc",
      "recipient": "3PFTGLDvE7rQfWtgSzBt7NS4NXXMQ1gUufs",
      "assetId": null,
      "amount": 699900000,
      "feeAsset": null,
      "attachment": ""
    },
    {
      "type": 4,
      "id": "JDWMMFkoyDGheThW3TS9E4SezM7pnr36sFsD8jrTwum",
      "sender": "3PLj6XQWBYWqRMHq4VFUtr9u7sh6LdftmZi",
      "senderPublicKey": "3BpdcfFZLiF1ndDSqvR6mqjKceeJM8iCwWwpoQmkh3CL",
      "fee": 100000,
      "timestamp": 1521485639923,
      "signature": "32D15oiMuCcMrHyySHDMwiykMkqtfyFsvXiPo7eK5qnWFSn8RqekUfcbq9YyxuqXz3wGRsjmXYGSL7XwE4jdwDd7",
      "recipient": "3P9dUze9nHRdfoKhFrZYKdsSpwW9JoE6Mzf",
      "assetId": null,
      "amount": 1699900000,
      "feeAsset": null,
      "attachment": ""
    },
    {
      "type": 4,
      "id": "8k9x1TwqJ7eQG6sBrUzFwigeCt9MLH29KqjRvCeoMJG2",
      "sender": "3P28Lsv1Pxf63EnvqoymXwbhQ1GnFFH5s6C",
      "senderPublicKey": "EFeMcgEbcKiSzpWwwX8HxsV2xWLrVK5EreWJH5zUDMLg",
      "fee": 100000,
      "timestamp": 1521485641000,
      "signature": "2dGUm2jdJp9z5dVJWFWznzjfzu5uE74YpGF4aeeSYYpj91t9QZuJaVfAfu3QE74rDHyLTb5HkFeK8KKT1jbtkDaq",
      "recipient": "3PRDjxHwETEhYXM3tjVMU4oYhfj3dqT6Vuw",
      "assetId": "B1u2TBpTYHWCuMuKLnbQfLvdLJ3zjgPiy3iMS2TSYugZ",
      "amount": 240000000,
      "feeAsset": null,
      "attachment": ""
    },
    {
      "type": 4,
      "id": "HEwV7SqimfKptMLQq2uAgem4qRuK6vYykHnJ7bhroptb",
      "sender": "3PEruAtC1edYhUPNoNAerP5xjdVaQMDHkPP",
      "senderPublicKey": "6BrPDS9ueWm9f4YUHHAWwLrTzdhjrWSGhSVafpkdCH2h",
      "fee": 100000,
      "timestamp": 1521485646637,
      "signature": "3t9s2Fd1a3xadCC9WcAa1mCC6aK4Ap7a9V5TekDuiTDprKJjzUETBQu5KxFLacbsHBsWaZvTtoLNdeCdkYDykRCx",
      "recipient": "3P3Y5U5CbJNHfszLa9JUieR91Et14yuLsRs",
      "assetId": "4UY7UjzhRxKYyLh6mtiPkZpC73HFLE9DFNGKs7ju6Ai3",
      "amount": 332000000,
      "feeAsset": null,
      "attachment": "7SpcwQnSaZBCPK1zk8"
    },
    {
      "type": 4,
      "id": "VmRUq5rpnHiYKeK73J6YEkUpiNuGis4P9G9RqH667KF",
      "sender": "3P5DKriHBPkeUN1GJXsq5S2tPwXqxw2f1Nr",
      "senderPublicKey": "xQ4XUGFd4k7xLuByc6YEYgo64mfPPYDLcDTYb3YmTwk",
      "fee": 100000,
      "timestamp": 1521485648453,
      "signature": "3CLeYAMCnoDH5MpHRK16f6QocyamXGoT6BsWuVER9eZ66KcujZoYsQE9vi4wdaP6toFeTdZeEse6Pb3fSZWKPnTR",
      "recipient": "3PBkqPd2chKH7uHViB3qkjKyYAJCSrsahbt",
      "assetId": null,
      "amount": 7052700000,
      "feeAsset": null,
      "attachment": "38kPaAzmvrCC4cV8FXh63"
    },
    {
      "type": 4,
      "id": "TRvLCLfAx1z2nd5E3TZDYhupNpvHhZbQ1n6igLrvwNi",
      "sender": "3PQGPqT3bkPsx7D7QdckFG17xi3ZLVLTswg",
      "senderPublicKey": "77eXKkirWnnWDhV233o9wmwSDdnErRfVXQYMfPSmtT9Z",
      "fee": 100000,
      "timestamp": 1521485652642,
      "signature": "2kvs9uCGE1iTWaqksM7Ebv5Du3eT6WKAY57EtHQSzMxdyru5dKmh23ZjyegMLotvBpCiSeYY8iKohUSd88KuD7PR",
      "recipient": "3PKhcTt5dyUeb1DzBAgNvHo7imC5TCfzHm5",
      "assetId": null,
      "amount": 499900000,
      "feeAsset": null,
      "attachment": ""
    },
    {
      "type": 7,
      "id": "6q71yVNSTbZN5XpUmhriwzdh8Ce9kdtVYbqoU7H8jD3t",
      "sender": "3PJaDyprvekvPXPuAtxrapacuDJopgJRaU3",
      "senderPublicKey": "7kPFrHDiGw1rCm7LPszuECwWYL3dMf6iMifLRDJQZMzy",
      "fee": 300000,
      "timestamp": 1521485657407,
      "signature": "2Ra25U2qAbZokwEker3PEwZ8qeiB3kjrMU1YURerFSKQWm6Mq3d2ajw3i1ASCjybAoZSDF4gsdbX16Fxb1tWZWrr",
      "order1": {
        "id": "2o5ac1VLW7cpHGzgQ9LT7N6RjEVkyEFrzW8oaUURxB7g",
        "senderPublicKey": "Gk7ZjMW7u39APyTYBTtiqkAWW8QPUkTXAzkqzcQ8ezFq",
        "matcherPublicKey": "7kPFrHDiGw1rCm7LPszuECwWYL3dMf6iMifLRDJQZMzy",
        "assetPair": {
          "amountAsset": "HZk1mbfuJpmxU1Fs4AX5MWLVYtctsNcg6e2C6VKqK8zk",
          "priceAsset": null
        },
        "orderType": "buy",
        "price": 3343543732,
        "amount": 975200000,
        "timestamp": 1521481403289,
        "expiration": 1523209403289,
        "matcherFee": 300000,
        "signature": "3gQN38GwH1CQrRSi274UoWVxxQhLV6k5gZX5gPNEEZRQKFXpdGg1R7YWoQXGaPBptaLbxJbQ3waajrUKcqHQmqx6"
      },
      "order2": {
        "id": "2itFT261iPB5odaB7pZCDrD3UFw3P3K5h6wTqS7trpYJ",
        "senderPublicKey": "D3J7Ak2sdUozpbXzTnJKF1ADmTgiTDga7hyPPW6ChZeF",
        "matcherPublicKey": "7kPFrHDiGw1rCm7LPszuECwWYL3dMf6iMifLRDJQZMzy",
        "assetPair": {
          "amountAsset": "HZk1mbfuJpmxU1Fs4AX5MWLVYtctsNcg6e2C6VKqK8zk",
          "priceAsset": null
        },
        "orderType": "sell",
        "price": 3343396882,
        "amount": 16559717,
        "timestamp": 1521485655519,
        "expiration": 1521485955519,
        "matcherFee": 300000,
        "signature": "4HGbej9mZQitMCBgBqE1UyRDkjAfxibZgjRugLe2fcLVnrnaKuB5kpk57VzNqENMxf7VrGtp9XfVQhz5JA6LtLi3"
      },
      "price": 3343543732,
      "amount": 16559717,
      "buyMatcherFee": 5094,
      "sellMatcherFee": 300000
    },
    {
      "type": 4,
      "id": "2H8jGRhL3PjhkCYhR2qP19iQzTUhiZ1DxDZvadTUfNcL",
      "sender": "3P98H6rC6bVVL92PgJAkZJnBCJWiQAgr5pQ",
      "senderPublicKey": "FTuQnxparWgo3GHLanjg4xe71C6wjcVRfiw12TPYqEoE",
      "fee": 100000,
      "timestamp": 1521485661052,
      "signature": "458vfNMoLE541YuDg4s1QA1k1VjjnCEnmfas8LDmEE4f5kV4iM9Bjxw2nUHnMFNXYvnpeiW2m27qTbjpqWRXbHu9",
      "recipient": "3PKzP42Cuz9pLgQPukHYQ6XVopBzbzfnbTd",
      "assetId": null,
      "amount": 200000000,
      "feeAsset": null,
      "attachment": ""
    },
    {
      "type": 7,
      "id": "Gn6295CDG165av49qnyhFi6m4tpVHAJRgixk1qcTREvf",
      "sender": "3PJaDyprvekvPXPuAtxrapacuDJopgJRaU3",
      "senderPublicKey": "7kPFrHDiGw1rCm7LPszuECwWYL3dMf6iMifLRDJQZMzy",
      "fee": 300000,
      "timestamp": 1521485676758,
      "signature": "3mkawAHUDcUr45Ee9QfUUb8v84cvRrkNVmNodamSpKKacCNvpEmDzHnL5FYFz1NQxARQtUGJfGAayYM6a7XSMVoy",
      "order1": {
        "id": "2pCsKwhrMcjgZYYr7QSf3KEwrExP2UbcUNWhYex41oMc",
        "senderPublicKey": "3AgmQx7g7YLYrodFVhQVuH9o9GXqQuQ36rfX4VmLUCAa",
        "matcherPublicKey": "7kPFrHDiGw1rCm7LPszuECwWYL3dMf6iMifLRDJQZMzy",
        "assetPair": {
          "amountAsset": "8LQW8f7P5d5PZM7GtZEBgaqRPGSzS3DfPuiXrURJ4AJS",
          "priceAsset": "2mX5DzVKWrAJw8iwdJnV2qtoeVG9h5nTDpTqC1wb1WEN"
        },
        "orderType": "buy",
        "price": 3413853,
        "amount": 5680486,
        "timestamp": 1521485676130,
        "expiration": 1521485746130,
        "matcherFee": 300000,
        "signature": "4f3oc3AiAJiYxveTidAnSaJVYWTm8UHutQctFm9qzdiUNwBCDdYUrTVQNX8mJJXkRyEE1nTfRMSWyEf61FUAHQBY"
      },
      "order2": {
        "id": "FLutNg1LS2XEKDFrutZ1S8jxiLXEXCwD7XKmMKP9fzc3",
        "senderPublicKey": "3AgmQx7g7YLYrodFVhQVuH9o9GXqQuQ36rfX4VmLUCAa",
        "matcherPublicKey": "7kPFrHDiGw1rCm7LPszuECwWYL3dMf6iMifLRDJQZMzy",
        "assetPair": {
          "amountAsset": "8LQW8f7P5d5PZM7GtZEBgaqRPGSzS3DfPuiXrURJ4AJS",
          "priceAsset": "2mX5DzVKWrAJw8iwdJnV2qtoeVG9h5nTDpTqC1wb1WEN"
        },
        "orderType": "sell",
        "price": 3413853,
        "amount": 5680486,
        "timestamp": 1521485673694,
        "expiration": 1521485743694,
        "matcherFee": 300000,
        "signature": "VRsSxmmrRwbjNPDW6CMtnBb6Fo8LQBVp5JWwpLVfKzbtShY9N4TNjcoCyYn5aj5w7nSzqxkfEgkbtKQvGBuUAkA"
      },
      "price": 3413853,
      "amount": 5680486,
      "buyMatcherFee": 300000,
      "sellMatcherFee": 300000
    },
    {
      "type": 4,
      "id": "3KYwvyDkiBC7PAP77TNSn5QMHskj8vL78DXpsCuuBwRe",
      "sender": "3PCHKUWPjBaTwrC35tYqXXY4Jakk4hR4nEa",
      "senderPublicKey": "9tbZhD3f9fjDQUnTXxjBXTp6VqaFm39y1akV161ZisGb",
      "fee": 100000,
      "timestamp": 1521485679561,
      "signature": "35mGZ7uoWWwUBu4fry1k1vMBm4yVCUv3dth3hLxmG732sEzQKXbM5c5YkHBSogBo7DuAWb6vCdahYmVFxa9Py44F",
      "recipient": "3PCsvPEYKnMn1tb7zS8akVSToA7Zv3mFKgg",
      "assetId": null,
      "amount": 200000000,
      "feeAsset": null,
      "attachment": ""
    },
    {
      "type": 7,
      "id": "5rADJNexzJFjjdPNbt3P6DV6FGagh16wG2rxuMM9LRDk",
      "sender": "3PJaDyprvekvPXPuAtxrapacuDJopgJRaU3",
      "senderPublicKey": "7kPFrHDiGw1rCm7LPszuECwWYL3dMf6iMifLRDJQZMzy",
      "fee": 300000,
      "timestamp": 1521485685614,
      "signature": "5vJcLPEjEE8dbYPhpmoig2DPffXVXLh5meU7AmJooDnBjxHMB5PhUc1rukGb6zsHpKNafqNwtP5rwQ15iFAVzDzh",
      "order1": {
        "id": "8kHMa9ef5TNWgQvSyaBYqUFBhCGqogCTyLSfMqvFzCDf",
        "senderPublicKey": "4JHdwwynumnF5Rr1mNKMCK2NxQEfaYVEAozTLLDrPSR3",
        "matcherPublicKey": "7kPFrHDiGw1rCm7LPszuECwWYL3dMf6iMifLRDJQZMzy",
        "assetPair": {
          "amountAsset": "GDbV3k7CbGjUWqWrdFLvVkqR9xdAzbjdhszsquf6Hdjd",
          "priceAsset": null
        },
        "orderType": "buy",
        "price": 339999,
        "amount": 600000000000,
        "timestamp": 1521485684657,
        "expiration": 1523213684657,
        "matcherFee": 300000,
        "signature": "866uZaoqjBKn8qPQ6y8dJmp6v8TswK6mm3FiVN3R1eYs5A8ezWTuGZr9m1GLJcafufCsKJE2xZTqovjMvABdnaX"
      },
      "order2": {
        "id": "EJ9vLL24QUkQgWjpY9WAMvKs8aeWbiFMXgW532WoXMeB",
        "senderPublicKey": "5HGEHGccJSiBviQrHgeTnWgkkFSzwwkWqkH637uXP6G2",
        "matcherPublicKey": "7kPFrHDiGw1rCm7LPszuECwWYL3dMf6iMifLRDJQZMzy",
        "assetPair": {
          "amountAsset": "GDbV3k7CbGjUWqWrdFLvVkqR9xdAzbjdhszsquf6Hdjd",
          "priceAsset": null
        },
        "orderType": "sell",
        "price": 339999,
        "amount": 80000000000,
        "timestamp": 1521481192490,
        "expiration": 1523209192490,
        "matcherFee": 300000,
        "signature": "2VL2hEZGDCBCBNj7bMxqgDyWJh12xAKnfy9EcXpgFEqquf3akoWyv7JZUtwTnsoqjV7Aw8gRhUYuET3Kiou3AM1q"
      },
      "price": 339999,
      "amount": 80000000000,
      "buyMatcherFee": 40000,
      "sellMatcherFee": 300000
    },
    {
      "type": 4,
      "id": "4pPTf7ig1jrkoZjivrGi1Y3yKHiVFaYC4QLfkefF8S9r",
      "sender": "3PDXgFsKxnRpKR2qhR3utLydVzgW6spy5Ui",
      "senderPublicKey": "24geotRFVZyNqgXjEEeZTCqF7rrLuE5krqhVVLtoo9vq",
      "fee": 100000,
      "timestamp": 1521485685526,
      "signature": "4LupK6LKLesZgbABVf5boc1GZkVR4PM3H2GC3tcZoi5SY2xHigsqxWC6ReYGcvxtthks9QRXFJMNtpc3g8pSw2ZC",
      "recipient": "3P53pHcJRb3PmvLtnGZyxPW1r38RTjgYoov",
      "assetId": "BmcArNN9VnKAp3HbvpKaoE3utwEXqvP1UjunS9DVKdGS",
      "amount": 1000000000,
      "feeAsset": null,
      "attachment": "4D2WC2Vr46GJXjwqHqPPjNqG4NDA14c31UQHLZDui7TDERP6XfQRug242rSZt4nmVH"
    }
  ],
  "height": 924610
}`
