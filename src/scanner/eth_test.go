package scanner

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/boltdb/bolt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/require"

	"github.com/skycoin/teller/src/util/dbutil"
	"github.com/skycoin/teller/src/util/testutil"
)

var (
	dummyEthBlocksBktName = []byte("blocks")
	errNoEthBlockHash     = errors.New("no block found for height")
)

type dummyEthrpcclient struct {
	db                           *bolt.DB
	blockHashes                  map[int64]string
	blockCount                   int64
	blockCountError              error
	blockVerboseTxError          error
	blockVerboseTxErrorCallCount int
	blockVerboseTxCallCount      int

	// used for testEthScannerBlockNextHashAppears
	blockNextHashMissingOnceAt int64
	hasSetMissingHash          bool
}

//types.Block hasn't serializd method with readable, so build this struct
//content is identical with types.Block
type BlockReadable struct {
	Header       *types.Header
	Uncles       []*types.Header
	Transactions types.Transactions
	Hash         common.Hash
}

func NewBlockFromBlockReadable(value []byte) (*types.Block, error) {
	var br BlockReadable
	if err := json.Unmarshal(value, &br); err != nil {
		return nil, err
	}

	anBlock := types.NewBlockWithHeader(br.Header)
	newBlock := anBlock.WithBody(br.Transactions, br.Uncles)
	return newBlock, nil
}

func openDummyEthDB(t *testing.T) *bolt.DB {
	// Blocks 2325205 through 2325214 are stored in this DB
	db, err := bolt.Open("./eth.db", 0600, nil)
	require.NoError(t, err)
	return db
}

func newDummyEthrpcclient(db *bolt.DB) *dummyEthrpcclient {
	return &dummyEthrpcclient{
		db:          db,
		blockHashes: make(map[int64]string),
	}
}

func (dec *dummyEthrpcclient) Shutdown() {
}

func (dec *dummyEthrpcclient) GetBlockVerbose(seq uint64) (*types.Block, error) {
	return dec.GetBlockVerboseTx(seq)
}

func (dec *dummyEthrpcclient) GetBlockVerboseTx(seq uint64) (*types.Block, error) {
	dec.blockVerboseTxCallCount++
	if dec.blockVerboseTxCallCount == dec.blockVerboseTxErrorCallCount {
		return nil, dec.blockVerboseTxError
	}

	var block *types.Block
	if err := dec.db.View(func(tx *bolt.Tx) error {
		return dbutil.ForEach(tx, dummyEthBlocksBktName, func(k, v []byte) error {
			tmpBlock, err := NewBlockFromBlockReadable(v)
			if err != nil {
				return err
			}
			if tmpBlock.NumberU64() == seq { //find corrsponding block
				block = tmpBlock
			}
			return nil
		})
	}); err != nil {
		return nil, err
	}
	if block == nil {
		return nil, errors.New("no block found for height")
	}

	if block.NumberU64() == uint64(dec.blockNextHashMissingOnceAt) && !dec.hasSetMissingHash {
		dec.hasSetMissingHash = true
	}

	if block.NumberU64() > uint64(dec.blockCount) {
		panic("scanner should not be scanning blocks past the blockCount height")
	}

	return block, nil
}

func (dec *dummyEthrpcclient) GetBlockCount() (int64, error) {
	if dec.blockCountError != nil {
		// blockCountError is only returned once
		err := dec.blockCountError
		dec.blockCountError = nil
		return 0, err
	}

	return dec.blockCount, nil
}

func setupEthScannerWithDB(t *testing.T, ethDB *bolt.DB, db *bolt.DB) *ETHScanner {
	log, _ := testutil.NewLogger(t)

	// Blocks 2325205 through 2325214 are stored in eth.db
	// Refer to https://etherscan.io/ or another explorer to see the block data
	rpc := newDummyEthrpcclient(ethDB)

	// The hash of the initial scan block needs to be set. The others don't
	//rpc.blockHashes[2325205] = "0xf2139b98f24f856f92f421a3bf9e5230e6426fc64d562b8a44f20159d561ca7c"

	// 2325214 is the highest block in the test data eth.db
	rpc.blockCount = 2325214

	store, err := NewStore(log, db)
	require.NoError(t, err)
	err = store.AddSupportedCoin(CoinTypeETH)
	require.NoError(t, err)

	cfg := Config{
		ScanPeriod:            time.Millisecond * 10,
		DepositBufferSize:     5,
		InitialScanHeight:     2325205,
		ConfirmationsRequired: 0,
	}
	scr, err := NewETHScanner(log, store, rpc, cfg)
	require.NoError(t, err)

	return scr
}

func setupEthScannerWithNonExistInitHeight(t *testing.T, ethDB *bolt.DB, db *bolt.DB) *ETHScanner {
	log, _ := testutil.NewLogger(t)

	// Blocks 2325205 through 2325214 are stored in eth.db
	// Refer to https://blockchain.info or another explorer to see the block data
	rpc := newDummyEthrpcclient(ethDB)

	// 2325214 is the highest block in the test data eth.db
	rpc.blockCount = 2325214

	store, err := NewStore(log, db)
	require.NoError(t, err)
	err = store.AddSupportedCoin(CoinTypeETH)
	require.NoError(t, err)

	// Block 2325204 doesn't exist in db
	cfg := Config{
		ScanPeriod:            time.Millisecond * 10,
		DepositBufferSize:     5,
		InitialScanHeight:     2325204,
		ConfirmationsRequired: 0,
	}
	scr, err := NewETHScanner(log, store, rpc, cfg)
	require.NoError(t, err)

	return scr
}

func setupEthScanner(t *testing.T, ethDB *bolt.DB) (*ETHScanner, func()) {
	db, shutdown := testutil.PrepareDB(t)

	scr := setupEthScannerWithDB(t, ethDB, db)

	return scr, shutdown
}

func testEthScannerRunProcessedLoop(t *testing.T, scr *ETHScanner, nDeposits int) {
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
				require.Equal(t, CoinTypeETH, d.CoinType)
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

func testEthScannerRun(t *testing.T, scr *ETHScanner) {
	nDeposits := 0

	// This address has 0 deposits
	err := scr.AddScanAddress("0x2cf014d432e92685ef1cf7bc7967a4e4debca092", CoinTypeETH)
	require.NoError(t, err)
	nDeposits = nDeposits + 0

	// This address has:
	// 1 deposit, in block 2325212
	err = scr.AddScanAddress("0x87b127ee022abcf9881b9bad6bb6aac25229dff0", CoinTypeETH)
	require.NoError(t, err)
	nDeposits = nDeposits + 2

	// This address has:
	// 9 deposits in block 2325213
	err = scr.AddScanAddress("0xbfc39b6f805a9e40e77291aff27aee3c96915bdd", CoinTypeETH)
	require.NoError(t, err)
	nDeposits = nDeposits + 9

	// Make sure that the deposit buffer size is less than the number of deposits,
	// to test what happens when the buffer is full
	require.True(t, scr.Base.(*BaseScanner).Cfg.DepositBufferSize < nDeposits)

	testEthScannerRunProcessedLoop(t, scr, nDeposits)
}

func testEthScannerRunProcessDeposits(t *testing.T, ethDB *bolt.DB) {
	// Tests that the scanner will scan multiple blocks sequentially, finding
	// all relevant deposits and adding them to the depositC channel.
	// All deposits on the depositC channel will be successfully processed
	// by the channel reader, and the scanner will mark these deposits as
	// "processed".
	scr, shutdown := setupEthScanner(t, ethDB)
	defer shutdown()

	testEthScannerRun(t, scr)
}

func testEthScannerGetBlockCountErrorRetry(t *testing.T, ethDB *bolt.DB) {
	// Test that if the scanner scan loop encounters an error when calling
	// GetBlockCount(), the loop continues to work fine
	// This test is that same as testEthScannerRunProcessDeposits,
	// except that the dummyEthrpcclient is configured to return an error
	// from GetBlockCount() one time
	scr, shutdown := setupEthScanner(t, ethDB)
	defer shutdown()

	scr.ethClient.(*dummyEthrpcclient).blockCountError = errors.New("block count error")

	testEthScannerRun(t, scr)
}

func testEthScannerConfirmationsRequired(t *testing.T, ethDB *bolt.DB) {
	// Test that the scanner uses cfg.ConfirmationsRequired correctly
	scr, shutdown := setupEthScanner(t, ethDB)
	defer shutdown()

	// Scanning starts at block 2325212, set the blockCount height to 1
	// confirmations higher, so that only block 2325212 is processed.
	scr.Base.(*BaseScanner).Cfg.ConfirmationsRequired = 1
	scr.ethClient.(*dummyEthrpcclient).blockCount = 2325214

	// Add scan addresses for blocks 2325205-2325214, but only expect to scan
	// deposits from block 2325205-2325212, since 2325213 and 2325214 don't have enough
	// confirmations
	nDeposits := 0

	// This address has:
	// 2 deposits in block 2325212
	// Only blocks 2325212  are processed, because blockCount is set
	// to 2325214 and the confirmations required is set to 1
	err := scr.AddScanAddress("0x87b127ee022abcf9881b9bad6bb6aac25229dff0", CoinTypeETH)
	require.NoError(t, err)
	nDeposits = nDeposits + 2

	// has't enough deposit
	require.True(t, scr.Base.(*BaseScanner).Cfg.DepositBufferSize > nDeposits)

	testEthScannerRunProcessedLoop(t, scr, nDeposits)
}

func testEthScannerScanBlockFailureRetry(t *testing.T, ethDB *bolt.DB) {
	// Test that when scanBlock() fails, it logs "Scan block failed"
	// and retries scan of the same block after ScanPeriod elapses.
	scr, shutdown := setupEthScanner(t, ethDB)
	defer shutdown()

	// Return an error on the 2nd call to GetBlockVerboseTx
	scr.ethClient.(*dummyEthrpcclient).blockVerboseTxError = errors.New("get block verbose tx error")
	scr.ethClient.(*dummyEthrpcclient).blockVerboseTxErrorCallCount = 2

	testEthScannerRun(t, scr)
}

func testEthScannerBlockNextHashAppears(t *testing.T, ethDB *bolt.DB) {
	// Test that when a block has no NextHash, the scanner waits until it has
	// one, then resumes normally
	scr, shutdown := setupEthScanner(t, ethDB)
	defer shutdown()

	// The block at height 2325208 will lack a NextHash one time
	// The scanner will continue and process everything normally
	scr.ethClient.(*dummyEthrpcclient).blockNextHashMissingOnceAt = 2325208

	testEthScannerRun(t, scr)
}

func testEthScannerDuplicateDepositScans(t *testing.T, ethDB *bolt.DB) {
	// Test that rescanning the same blocks doesn't send extra deposits
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	nDeposits := 0

	// This address has:
	// 2 deposit, in block 2325212
	scr := setupEthScannerWithDB(t, ethDB, db)
	err := scr.AddScanAddress("0x87b127ee022abcf9881b9bad6bb6aac25229dff0", CoinTypeETH)
	require.NoError(t, err)
	nDeposits = nDeposits + 2

	testEthScannerRunProcessedLoop(t, scr, nDeposits)

	// Scanning again will have no new deposits
	scr = setupEthScannerWithDB(t, ethDB, db)
	testEthScannerRunProcessedLoop(t, scr, 0)
}

func testEthScannerLoadUnprocessedDeposits(t *testing.T, ethDB *bolt.DB) {
	// Test that pending unprocessed deposits from the db are loaded when
	// then scanner starts.
	scr, shutdown := setupEthScanner(t, ethDB)
	defer shutdown()

	// NOTE: This data is fake, but the addresses and Txid are valid
	unprocessedDeposits := []Deposit{
		{
			CoinType:  CoinTypeETH,
			Address:   "0x196736a260c6e7c86c88a73e2ffec400c9caef71",
			Value:     1e8,
			Height:    2325212,
			Tx:        "0xc724f4aae6f89e6296aec22c6795e7423b6776e2ee3c5f942cf3817a9ded0c32",
			N:         1,
			Processed: false,
		},
		{
			CoinType:  CoinTypeETH,
			Address:   "0x2a5ee9b4307a0030982ed00ca7e904a20fc53a12",
			Value:     10e8,
			Height:    2325212,
			Tx:        "0xca8d662c6cf2dcd0e8c9075b58bfbfa7ee4769e5efd6f45e490309d58074913e",
			N:         1,
			Processed: false,
		},
	}

	processedDeposit := Deposit{
		CoinType:  CoinTypeETH,
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
	testEthScannerRunProcessedLoop(t, scr, len(unprocessedDeposits))
}

func testEthScannerProcessDepositError(t *testing.T, ethDB *bolt.DB) {
	// Test that when processDeposit() fails, the deposit is NOT marked as processed
	scr, shutdown := setupEthScanner(t, ethDB)
	defer shutdown()

	nDeposits := 0

	// This address has:
	// 9 deposits in block 2325213
	err := scr.AddScanAddress("0xbfc39b6f805a9e40e77291aff27aee3c96915bdd", CoinTypeETH)
	require.NoError(t, err)
	nDeposits = nDeposits + 9

	// Make sure that the deposit buffer size is less than the number of deposits,
	// to test what happens when the buffer is full
	require.True(t, scr.Base.(*BaseScanner).Cfg.DepositBufferSize < nDeposits)

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
				require.Equal(t, CoinTypeETH, d.CoinType)
				require.Equal(t, "0xbfc39b6f805a9e40e77291aff27aee3c96915bdd", d.Address)
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

func testEthScannerInitialGetBlockHashError(t *testing.T, ethDB *bolt.DB) {
	// Test that scanner.Run() returns an error if the initial GetBlockHash
	// based upon scanner.Base.Cfg.InitialScanHeight fails
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	scr := setupEthScannerWithNonExistInitHeight(t, ethDB, db)

	err := scr.Run()
	require.Error(t, err)
	require.Equal(t, errNoEthBlockHash, err)
}

func TestEthScanner(t *testing.T) {
	ethDB := openDummyEthDB(t)
	defer testutil.CheckError(t, ethDB.Close)
	t.Run("group", func(t *testing.T) {

		t.Run("RunProcessDeposits", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testEthScannerRunProcessDeposits(t, ethDB)
		})

		t.Run("GetBlockCountErrorRetry", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testEthScannerGetBlockCountErrorRetry(t, ethDB)
		})

		t.Run("InitialGetBlockHashError", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testEthScannerInitialGetBlockHashError(t, ethDB)
		})

		t.Run("ProcessDepositError", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testEthScannerProcessDepositError(t, ethDB)
		})

		t.Run("ConfirmationsRequired", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testEthScannerConfirmationsRequired(t, ethDB)
		})

		t.Run("ScanBlockFailureRetry", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testEthScannerScanBlockFailureRetry(t, ethDB)
		})

		t.Run("LoadUnprocessedDeposits", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testEthScannerLoadUnprocessedDeposits(t, ethDB)
		})

		t.Run("DuplicateDepositScans", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testEthScannerDuplicateDepositScans(t, ethDB)
		})

		t.Run("BlockNextHashAppears", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testEthScannerBlockNextHashAppears(t, ethDB)
		})
	})
}
