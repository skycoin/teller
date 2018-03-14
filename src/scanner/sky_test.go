package scanner

import (
	"errors"
	"testing"
	"time"

	"github.com/boltdb/bolt"
	"github.com/stretchr/testify/require"

	"github.com/MDLlife/teller/src/util/dbutil"
	"github.com/MDLlife/teller/src/util/testutil"
	"fmt"
	"github.com/skycoin/skycoin/src/api/cli"
	"github.com/skycoin/skycoin/src/visor"

	"github.com/skycoin/skycoin/src/api/webrpc"
	"github.com/sirupsen/logrus"
	"github.com/skycoin/skycoin/src/coin"
	"github.com/skycoin/skycoin/src/visor/historydb"
	"encoding/json"
	"github.com/skycoin/skycoin/src/daemon"
	"encoding/hex"
	"github.com/skycoin/skycoin/src/cipher"
	mock "github.com/stretchr/testify/mock"

)

var (
	dummySkyBlocksBktName = []byte("blocks")
	errNoSkyBlockHash     = errors.New("no block found for height")
	testWebRPCAddr = "127.0.0.1:8081"
	txHeight    = uint64(103)
	txConfirmed = true
)

type fakeGateway struct {
	transactions         map[string]string
	injectRawTxMap       map[string]bool // key: transaction hash, value indicates whether the injectTransaction should return error.
	injectedTransactions map[string]string
	addrRecvUxOuts       []*historydb.UxOut
	addrSpentUxOUts      []*historydb.UxOut
	uxouts               []coin.UxOut
}

// GatewayerMock mock
type GatewayerMock struct {
	mock.Mock
}

func NewGatewayerMock() *GatewayerMock {
	return &GatewayerMock{}
}

type dummySkyrpcclient struct {
	db                           *bolt.DB
	blockHashes                  map[int64]string
	blockCount                   int64
	blockCountError              error
	blockVerboseTxError          error
	blockVerboseTxErrorCallCount int
	blockVerboseTxCallCount      int

	// used for testSkyScannerBlockNextHashAppears
	blockNextHashMissingOnceAt int64
	hasSetMissingHash          bool

	log       logrus.FieldLogger
	Base      CommonScanner
	walletFile string
	changeAddr string
	skyRpcClient  *webrpc.Client
}


//func NewSKYBlockFromBlockReadable(value []byte) (*visor.ReadableBlock, error) {
//	var br BlockReadable
//	if err := json.Unmarshal(value, &br); err != nil {
//		return nil, err
//	}
//
//	//anBlock := types.NewBlockWithHeader(br.Header)
//	//newBlock := anBlock.WithBody(br.Transactions, br.Uncles)
//
//
//
//	return newBlock, nil
//}

func openDummySkyDB(t *testing.T) *bolt.DB {
	// Blocks 2325205 through 2325214 are stored in this DB
	db, err := bolt.Open("./sky.db", 0600, nil)
	require.NoError(t, err)
	return db
}

func newDummySkyrpcclient(t *testing.T, db *bolt.DB) *dummySkyrpcclient {
	rpc, err := webrpc.New(testWebRPCAddr, &fakeGateway{})
	require.NoError(t, err)
	rpc.WorkerNum = 1
	rpc.ChanBuffSize = 2

	client := &webrpc.Client{
		Addr: rpc.Addr,
	}

	rpc.WorkerNum = 1
	rpc.ChanBuffSize = 2
	return &dummySkyrpcclient{
		db:          db,
		blockHashes: make(map[int64]string),
		skyRpcClient: client,
	}
}

// Send sends coins to recv address
func (c *dummySkyrpcclient) Send(recvAddr string, amount uint64) (string, error) {
	// validate the recvAddr
	if _, err := cipher.DecodeBase58Address(recvAddr); err != nil {
		return "", err
	}

	if amount == 0 {
		return "", fmt.Errorf("Can't send 0 coins", amount)
	}

	sendAmount := cli.SendAmount{
		Addr:  recvAddr,
		Coins: amount,
	}

	return cli.SendFromWallet(c.skyRpcClient, c.walletFile, c.changeAddr, []cli.SendAmount{sendAmount})
}

// GetTransaction returns transaction by txid
func (c *dummySkyrpcclient) GetTransaction(txid string) (*webrpc.TxnResult, error) {
	return c.skyRpcClient.GetTransactionByID(txid)
}

func (c *dummySkyrpcclient) GetBlocks(start, end uint64) (*visor.ReadableBlocks, error) {
	param := []uint64{start, end}
	blocks := visor.ReadableBlocks{}

	if err := c.skyRpcClient.Do(&blocks, "get_blocks", param); err != nil {
		return nil, err
	}

	return &blocks, nil
}

func (c *dummySkyrpcclient) GetBlocksBySeq(seq uint64) (*visor.ReadableBlock, error) {
	blocks := decodeBlock(blockString)
	if len(blocks.Blocks) == 0 {
		return nil, nil
	}
	return &blocks.Blocks[0], nil
}


func (c *dummySkyrpcclient) GetBlockCount(seq uint64) (*visor.ReadableBlock, error) {
	blocks := decodeBlock(blockString)
	if len(blocks.Blocks) == 0 {
		return nil, nil
	}
	return &blocks.Blocks[0], nil
}

func (c *dummySkyrpcclient) GetLastBlocks() (*visor.ReadableBlock, error) {
	blocks := decodeBlock(blockString)
	if len(blocks.Blocks) == 0 {
		return nil, nil
	}
	return &blocks.Blocks[0], nil
}

func (c *dummySkyrpcclient) Shutdown() {
}

// Send sends coins to batch recv address
func (c *dummySkyrpcclient) SendBatch(saList []cli.SendAmount) (string, error) {
	// validate the recvAddr
	return "", nil
}


func setupSkyScannerWithNonExistInitHeight(t *testing.T, ethDB *bolt.DB, db *bolt.DB) *SKYScanner {
	log, _ := testutil.NewLogger(t)

	// Blocks 2325205 through 2325214 are stored in eth.db
	// Refer to https://blockchain.info or another explorer to see the block data
	rpc := newDummySkyrpcclient(t,ethDB)

	// 2325214 is the highest block in the test data eth.db
	rpc.blockCount = 2325214

	store, err := NewStore(log, db)
	require.NoError(t, err)
	err = store.AddSupportedCoin(CoinTypeSKY)
	require.NoError(t, err)

	// Block 2325204 doesn't exist in db
	cfg := Config{
		ScanPeriod:            time.Millisecond * 10,
		DepositBufferSize:     5,
		InitialScanHeight:     2325204,
		ConfirmationsRequired: 0,
	}
	scr, err := NewSkycoinScanner(log, store, rpc, cfg)
	require.NoError(t, err)

	return scr
}


func setupSkyScannerWithDB(t *testing.T, skyDB *bolt.DB, db *bolt.DB) *SKYScanner {
	log, _ := testutil.NewLogger(t)

	// Blocks 2325205 through 2325214 are stored in eth.db
	// Refer to https://etherscan.io/ or another explorer to see the block data
	rpc := newDummySkyrpcclient(t, skyDB)

	// The hash of the initial scan block needs to be set. The others don't
	//rpc.blockHashes[2325205] = "0xf2139b98f24f856f92f421a3bf9e5230e6426fc64d562b8a44f20159d561ca7c"

	// 2325214 is the highest block in the test data eth.db
	rpc.blockCount = 180

	store, err := NewStore(log, db)
	require.NoError(t, err)
	err = store.AddSupportedCoin(CoinTypeSKY)
	require.NoError(t, err)

	cfg := Config{
		ScanPeriod:            time.Millisecond * 10,
		DepositBufferSize:     5,
		InitialScanHeight:     1,
		ConfirmationsRequired: 0,
	}
	scr, err := NewSkycoinScanner(log, store, rpc, cfg)
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
		err := scr.Base.GetStorer().(*Store).db.View(func(tx *bolt.Tx) error {
			for _, dv := range dvs {
				var d Deposit
				err := dbutil.GetBucketObject(tx, DepositBkt, dv.ID(), &d)
				require.NoError(t, err)
				if err != nil {
					return err
				}

				require.True(t, d.Processed)
				require.Equal(t, CoinTypeSKY, d.CoinType)
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

func testSkyScannerRun(t *testing.T, scr *SKYScanner) {
	nDeposits := 0

	// This address has 0 deposits
	err := scr.AddScanAddress("0x2cf014d432e92685ef1cf7bc7967a4e4debca092", CoinTypeSKY)
	require.NoError(t, err)
	nDeposits = nDeposits + 0

	// This address has:
	// 1 deposit, in block 2325212
	err = scr.AddScanAddress("0x87b127ee022abcf9881b9bad6bb6aac25229dff0", CoinTypeSKY)
	require.NoError(t, err)
	nDeposits = nDeposits + 2

	// This address has:
	// 9 deposits in block 2325213
	err = scr.AddScanAddress("0xbfc39b6f805a9e40e77291aff27aee3c96915bdd", CoinTypeSKY)
	require.NoError(t, err)
	nDeposits = nDeposits + 9

	// Make sure that the deposit buffer size is less than the number of deposits,
	// to test what happens when the buffer is full
	require.True(t, scr.Base.(*BaseScanner).Cfg.DepositBufferSize < nDeposits)

	testSkyScannerRunProcessedLoop(t, scr, nDeposits)
}

func testSkyScannerRunProcessDeposits(t *testing.T, ethDB *bolt.DB) {
	// Tests that the scanner will scan multiple blocks sequentially, finding
	// all relevant deposits and adding them to the depositC channel.
	// All deposits on the depositC channel will be successfully processed
	// by the channel reader, and the scanner will mark these deposits as
	// "processed".
	scr, shutdown := setupSkyScanner(t, ethDB)
	defer shutdown()

	testSkyScannerRun(t, scr)
}

func testSkyScannerGetBlockCountErrorRetry(t *testing.T, ethDB *bolt.DB) {
	// Test that if the scanner scan loop encounters an error when calling
	// GetBlockCount(), the loop continues to work fine
	// This test is that same as testSkyScannerRunProcessDeposits,
	// except that the dummySkyrpcclient is configured to return an error
	// from GetBlockCount() one time
	scr, shutdown := setupSkyScanner(t, ethDB)
	defer shutdown()

	scr.skyRpcClient.(*dummySkyrpcclient).blockCountError = errors.New("block count error")

	testSkyScannerRun(t, scr)
}

func testSkyScannerConfirmationsRequired(t *testing.T, ethDB *bolt.DB) {
	// Test that the scanner uses cfg.ConfirmationsRequired correctly
	scr, shutdown := setupSkyScanner(t, ethDB)
	defer shutdown()

	// Scanning starts at block 2325212, set the blockCount height to 1
	// confirmations higher, so that only block 2325212 is processed.
	scr.Base.(*BaseScanner).Cfg.ConfirmationsRequired = 1
	scr.skyRpcClient.(*dummySkyrpcclient).blockCount = 2325214

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

	testSkyScannerRunProcessedLoop(t, scr, nDeposits)
}

func testSkyScannerScanBlockFailureRetry(t *testing.T, ethDB *bolt.DB) {
	// Test that when scanBlock() fails, it logs "Scan block failed"
	// and retries scan of the same block after ScanPeriod elapses.
	scr, shutdown := setupSkyScanner(t, ethDB)
	defer shutdown()

	// Return an error on the 2nd call to GetBlockVerboseTx
	scr.skyRpcClient.(*dummySkyrpcclient).blockVerboseTxError = errors.New("get block verbose tx error")
	scr.skyRpcClient.(*dummySkyrpcclient).blockVerboseTxErrorCallCount = 2

	testSkyScannerRun(t, scr)
}

func testSkyScannerBlockNextHashAppears(t *testing.T, skyDB *bolt.DB) {
	// Test that when a block has no NextHash, the scanner waits until it has
	// one, then resumes normally
	scr, shutdown := setupSkyScanner(t, skyDB)
	defer shutdown()

	// The block at height 2325208 will lack a NextHash one time
	// The scanner will continue and process everything normally
	scr.skyRpcClient.(*dummySkyrpcclient).blockNextHashMissingOnceAt = 2325208

	testSkyScannerRun(t, scr)
}

func testSkyScannerDuplicateDepositScans(t *testing.T, skyDB *bolt.DB) {
	// Test that rescanning the same blocks doesn't send extra deposits
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	nDeposits := 0

	// This address has:
	// 2 deposit, in block 2325212
	scr := setupSkyScannerWithDB(t, skyDB, db)
	err := scr.AddScanAddress("0x87b127ee022abcf9881b9bad6bb6aac25229dff0", CoinTypeSKY)
	require.NoError(t, err)
	nDeposits = nDeposits + 2

	testSkyScannerRunProcessedLoop(t, scr, nDeposits)

	// Scanning again will have no new deposits
	scr = setupSkyScannerWithDB(t, skyDB, db)
	testSkyScannerRunProcessedLoop(t, scr, 0)
}

func testSkyScannerLoadUnprocessedDeposits(t *testing.T, ethDB *bolt.DB) {
	// Test that pending unprocessed deposits from the db are loaded when
	// then scanner starts.
	scr, shutdown := setupSkyScanner(t, ethDB)
	defer shutdown()

	// NOTE: This data is fake, but the addresses and Txid are valid
	unprocessedDeposits := []Deposit{
		{
			CoinType:  CoinTypeSKY,
			Address:   "0x196736a260c6e7c86c88a73e2ffec400c9caef71",
			Value:     1e8,
			Height:    2325212,
			Tx:        "0xc724f4aae6f89e6296aec22c6795e7423b6776e2ee3c5f942cf3817a9ded0c32",
			N:         1,
			Processed: false,
		},
		{
			CoinType:  CoinTypeSKY,
			Address:   "0x2a5ee9b4307a0030982ed00ca7e904a20fc53a12",
			Value:     10e8,
			Height:    2325212,
			Tx:        "0xca8d662c6cf2dcd0e8c9075b58bfbfa7ee4769e5efd6f45e490309d58074913e",
			N:         1,
			Processed: false,
		},
	}

	processedDeposit := Deposit{
		CoinType:  CoinTypeSKY,
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
	testSkyScannerRunProcessedLoop(t, scr, len(unprocessedDeposits))
}

func testSkyScannerProcessDepositError(t *testing.T, ethDB *bolt.DB) {
	// Test that when processDeposit() fails, the deposit is NOT marked as processed
	scr, shutdown := setupSkyScanner(t, ethDB)
	defer shutdown()

	nDeposits := 0

	// This address has:
	// 9 deposits in block 2325213
	err := scr.AddScanAddress("0xbfc39b6f805a9e40e77291aff27aee3c96915bdd", CoinTypeSKY)
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
				require.Equal(t, CoinTypeSKY, d.CoinType)
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

func testSkyScannerInitialGetBlockHashError(t *testing.T, ethDB *bolt.DB) {
	// Test that scanner.Run() returns an error if the initial GetBlockHash
	// based upon scanner.Base.Cfg.InitialScanHeight fails
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	scr := setupSkyScannerWithNonExistInitHeight(t, ethDB, db)

	err := scr.Run()
	require.Error(t, err)
	require.Equal(t, errNoSkyBlockHash, err)
}

func TestSkyScanner(t *testing.T) {
	ethDB := openDummySkyDB(t)
	defer testutil.CheckError(t, ethDB.Close)
	t.Run("group", func(t *testing.T) {

		t.Run("RunProcessDeposits", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testSkyScannerRunProcessDeposits(t, ethDB)
		})

		t.Run("GetBlockCountErrorRetry", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testSkyScannerGetBlockCountErrorRetry(t, ethDB)
		})

		t.Run("InitialGetBlockHashError", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testSkyScannerInitialGetBlockHashError(t, ethDB)
		})

		t.Run("ProcessDepositError", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testSkyScannerProcessDepositError(t, ethDB)
		})

		t.Run("ConfirmationsRequired", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testSkyScannerConfirmationsRequired(t, ethDB)
		})

		t.Run("ScanBlockFailureRetry", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testSkyScannerScanBlockFailureRetry(t, ethDB)
		})

		t.Run("LoadUnprocessedDeposits", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testSkyScannerLoadUnprocessedDeposits(t, ethDB)
		})

		t.Run("DuplicateDepositScans", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testSkyScannerDuplicateDepositScans(t, ethDB)
		})

		t.Run("BlockNextHashAppears", func(t *testing.T) {
			if parallel {
				t.Parallel()
			}
			testSkyScannerBlockNextHashAppears(t, ethDB)
		})
	})
}





func (fg fakeGateway) GetLastBlocks(num uint64) (*visor.ReadableBlocks, error) {
	var blocks visor.ReadableBlocks
	if err := json.Unmarshal([]byte(blockString), &blocks); err != nil {
		return nil, err
	}

	return &blocks, nil
}

func (fg fakeGateway) GetBlocks(start, end uint64) (*visor.ReadableBlocks, error) {
	var blocks visor.ReadableBlocks
	if start > end {
		return nil, nil
	}

	if err := json.Unmarshal([]byte(blockString), &blocks); err != nil {
		return nil, err
	}

	return &blocks, nil
}

func (fg fakeGateway) GetBlocksInDepth(vs []uint64) (*visor.ReadableBlocks, error) {
	return nil, nil
}

func (fg fakeGateway) GetUnspentOutputs(filters ...daemon.OutputsFilter) (visor.ReadableOutputSet, error) {
	outs := []coin.UxOut{}
	for _, f := range filters {
		outs = f(fg.uxouts)
	}

	headTime := uint64(time.Now().UTC().Unix())

	rbOuts, err := visor.NewReadableOutputs(headTime, outs)
	if err != nil {
		return visor.ReadableOutputSet{}, err
	}

	return visor.ReadableOutputSet{
		HeadOutputs: rbOuts,
	}, nil
}

func (fg fakeGateway) GetTransaction(txid cipher.SHA256) (*visor.Transaction, error) {
	str, ok := fg.transactions[txid.Hex()]
	if ok {
		return decodeRawTransaction(str), nil
	}
	return nil, nil
}

func (fg *fakeGateway) InjectTransaction(txn coin.Transaction) error {
	if _, v := fg.injectRawTxMap[txn.Hash().Hex()]; v {
		if fg.injectedTransactions == nil {
			fg.injectedTransactions = make(map[string]string)
		}
		fg.injectedTransactions[txn.Hash().Hex()] = hex.EncodeToString(txn.Serialize())
		return nil
	}

	return errors.New("fake gateway inject transaction failed")
}

func (fg fakeGateway) GetAddrUxOuts(addr cipher.Address) ([]*historydb.UxOutJSON, error) {
	return nil, nil
}

func (fg fakeGateway) GetTimeNow() uint64 {
	return 0
}

func decodeRawTransaction(rawTxStr string) *visor.Transaction {
	rawTx, err := hex.DecodeString(rawTxStr)
	if err != nil {
		panic(fmt.Sprintf("invalid raw transaction:%v", err))
	}

	tx := coin.MustTransactionDeserialize(rawTx)
	return &visor.Transaction{
		Txn: tx,
		Status: visor.TransactionStatus{
			Confirmed: txConfirmed,
			Height:    txHeight,
		},
	}
}


func decodeBlock(str string) *visor.ReadableBlocks {
	var blocks visor.ReadableBlocks
	if err := json.Unmarshal([]byte(str), &blocks); err != nil {
		panic(err)
	}
	return &blocks
}

var blockString = `{
    "blocks": [
        {
            "header": {
                "version": 0,
                "timestamp": 1477295242,
                "seq": 1,
                "fee": 20732,
                "prev_hash": "f680fe1f068a1cd5c3ef9194f91a9bc3cacffbcae4a32359a3c014da4ef7516f",
                "hash": "662835cc081e037561e1fe05860fdc4b426f6be562565bfaa8ec91be5675064a"
            },
            "body": {
                "txns": [
                    {
                        "length": 608,
                        "type": 0,
                        "txid": "662835cc081e037561e1fe05860fdc4b426f6be562565bfaa8ec91be5675064a",
                        "inner_hash": "37f1111bd83d9c995b9e48511bd52de3b0e440dccbf6d2cfd41dee31a10f1aa4",
                        "sigs": [
                            "ef0b8e1465557e6f21cb2bfad17136188f0b9bd54bba3db76c3488eb8bc900bc7662e3fe162dd6c236d9e52a7051a2133855081a91f6c1a63e1fce2ae9e3820e00",
                            "800323c8c22a2c078cecdfad35210902f91af6f97f0c63fe324e0a9c2159e9356f2fbbfff589edea5a5c24453ef5fc0cd5929f24bebee28e37057acd6d42f3d700",
                            "ca6a6ef5f5fb67490d88ddeeee5e5d11055246613b03e7ed2ad5cc82d01077d262e2da56560083928f5389580ae29500644719cf0e82a5bf065cecbed857598400",
                            "78ddc117607159c7b4c76fc91deace72425f21f2df5918d44d19a377da68cc610668c335c84e2bb7a8f16cd4f9431e900585fc0a3f1024b722b974fcef59dfd500",
                            "4c484d44072e23e97a437deb03a85e3f6eca0bd8875031efe833e3c700fc17f91491969b9864b56c280ef8a68d18dd728b211ce1d46fe477fe3104d73d55ad6501"
                        ],
                        "inputs": [
                            "4bd7c68ecf3039c2b2d8c26a5e2983e20cf53b6d62b099e7786546b3c3f600f9",
                            "f9e39908677cae43832e1ead2514e01eaae48c9a3614a97970f381187ee6c4b1",
                            "7e8ac23a2422b4666ff45192fe36b1bd05f1285cf74e077ac92cabf5a7c1100e",
                            "b3606a4f115d4161e1c8206f4fb5ac0e91551c40d0ee6fe40c86040d2faacac0",
                            "305f1983f5b630bba27e2777c229c725b6b57f37a6ddee138d1d82ae56311909"
                        ],
                        "outputs": [
                            {
                                "uxid": "574d7e5afaefe4ee7e0adf6ce1971d979f038adc8ebbd35771b2c19b0bad7e3d",
                                "dst": "cBnu9sUvv12dovBmjQKTtfE4rbjMmf3fzW",
                                "coins": "1",
                                "hours": 3455
                            },
                            {
                                "uxid": "6d8a9c89177ce5e9d3b4b59fff67c00f0471fdebdfbb368377841b03fc7d688b",
                                "dst": "fyqX5YuwXMUs4GEUE3LjLyhrqvNztFHQ4B",
                                "coins": "5",
                                "hours": 3455
                            }
                        ]
                    }
                ]
            }
        }
    ]
}`
