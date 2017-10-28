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

var dummyBlocksBktName = []byte("blocks")

type scannedBlock struct {
	Hash   string
	Height int32
}

type dummyBtcrpcclient struct {
	db              *bolt.DB
	bestBlock       scannedBlock
	blockHashes     map[int64]string
	blockCount      int64
	blockCountError error
}

func openDummyBtcDB(t *testing.T) *bolt.DB {
	// Blocks 235205 through 235212 are stored in this DB
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

func (dbc *dummyBtcrpcclient) GetBestBlock() (*chainhash.Hash, int32, error) {
	hash, err := chainhash.NewHashFromStr(dbc.bestBlock.Hash)
	if err != nil {
		return nil, 0, err
	}

	return hash, int32(dbc.bestBlock.Height), nil
}

func (dbc *dummyBtcrpcclient) GetBlockVerboseTx(hash *chainhash.Hash) (*btcjson.GetBlockVerboseResult, error) {
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

	return block, nil
}

func (dbc *dummyBtcrpcclient) GetBlockCount() (int64, error) {
	if dbc.blockCountError != nil {
		return 0, dbc.blockCountError
	}

	return dbc.blockCount, nil
}

func (dbc *dummyBtcrpcclient) GetBlockHash(height int64) (*chainhash.Hash, error) {
	hash := dbc.blockHashes[height]
	if hash == "" {
		return nil, fmt.Errorf("No block hash for height %d", height)
	}

	return chainhash.NewHashFromStr(hash)
}

func setupScanner(t *testing.T) (*BTCScanner, func()) {
	db, shutdown := testutil.PrepareDB(t)

	log, _ := testutil.NewLogger(t)

	btcDB := openDummyBtcDB(t)

	// Blocks 235205 through 235212 are stored in btc.db
	// Refer to https://blockchain.info or another explorer to see the block data
	rpc := newDummyBtcrpcclient(btcDB)
	rpc.blockHashes[235205] = "000000000000018d8ece83a004c5a919210d67798d13aa901c4d07f8bf87b719"

	rpc.bestBlock = scannedBlock{
		Hash:   "000000000000014e5217c81d6228a9274395a8bee3eb87277dd9e4315ee0f439",
		Height: 235207,
	}

	rpc.blockCount = 235300

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

	return scr, func() {
		shutdown()
		btcDB.Close()
	}
}

func TestScannerRunProcessDeposits(t *testing.T) {
	// Tests that the scanner will scan multiple blocks sequentially, finding
	// all relevant deposits and adding them to the depositC channel.
	// All deposits on the depositC channel will be successfully processed
	// by the channel reader, and the scanner will mark these deposits as
	// "processed".
	scr, shutdown := setupScanner(t)
	defer shutdown()

	nDeposits := 0

	// This address has 1 deposit, in block 235207
	err := scr.AddScanAddress("1LcEkgX8DCrQczLMVh9LDTRnkdVV2oun3A")
	require.NoError(t, err)
	nDeposits = nDeposits + 1

	// This address has 1 deposit, in block 235206
	err = scr.AddScanAddress("1N8G4JM8krsHLQZjC51R7ZgwDyihmgsQYA")
	require.NoError(t, err)
	nDeposits = nDeposits + 1

	// This address has 95 deposits, in block 235205
	err = scr.AddScanAddress("1LEkderht5M5yWj82M87bEd4XDBsczLkp9")
	require.NoError(t, err)
	nDeposits = nDeposits + 95

	// Make sure that the deposit buffer size is less than the number of deposits,
	// to test what happens when the buffer is full
	require.True(t, scr.cfg.DepositBufferSize < nDeposits)

	done := make(chan struct{})
	go func() {
		defer close(done)
		var dvs []DepositNote
		for dv := range scr.GetDeposit() {
			dvs = append(dvs, dv)
			dv.ErrC <- nil
		}

		require.Len(t, dvs, nDeposits)

		// check all deposits
		err := scr.store.(*Store).db.View(func(tx *bolt.Tx) error {
			for _, dv := range dvs {
				var d Deposit
				err := dbutil.GetBucketObject(tx, depositBkt, dv.TxN(), &d)
				require.NoError(t, err)
				require.True(t, d.Processed)
				if err != nil {
					return err
				}
			}

			return nil
		})
		require.NoError(t, err)
	}()

	time.AfterFunc(scr.cfg.ScanPeriod*time.Duration(nDeposits*2), func() {
		scr.Shutdown()
	})

	err = scr.Run()
	require.NoError(t, err)
	<-done
}

func TestScannerProcessDepositError(t *testing.T) {
	// Test that when processDeposit() fails, the deposit is NOT marked as processed
	scr, shutdown := setupScanner(t)
	defer shutdown()

	nDeposits := 0

	// This address has 95 deposits, in block 235205
	err := scr.AddScanAddress("1LEkderht5M5yWj82M87bEd4XDBsczLkp9")
	require.NoError(t, err)
	nDeposits = nDeposits + 95

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

		require.Len(t, dvs, nDeposits)

		// check all deposits, none should be marked as "Processed"
		err := scr.store.(*Store).db.View(func(tx *bolt.Tx) error {
			for _, dv := range dvs {
				var d Deposit
				err := dbutil.GetBucketObject(tx, depositBkt, dv.TxN(), &d)
				require.NoError(t, err)
				require.False(t, d.Processed)
				if err != nil {
					return err
				}
			}

			return nil
		})
		require.NoError(t, err)
	}()

	time.AfterFunc(scr.cfg.ScanPeriod*time.Duration(nDeposits*2), func() {
		scr.Shutdown()
	})

	err = scr.Run()
	require.NoError(t, err)
	<-done
}

func TestScannerLoadUnprocessedDeposits(t *testing.T) {
	// Test that pending unprocessed deposits from the db are loaded when
	// then scanner starts.
	scr, shutdown := setupScanner(t)
	defer shutdown()

	// NOTE: This data is fake, but the addresses and Txid are valid
	unprocessedDeposits := []Deposit{
		{
			Address:   "1LEkderht5M5yWj82M87bEd4XDBsczLkp9",
			Value:     1e8,
			Height:    23505,
			Tx:        "239e007dc20805add047d305cdfb87de1bae9bea1e47acbf58f38731ad58d70d",
			N:         1,
			Processed: false,
		},
		{
			Address:   "16Lr3Zhjjb7KxeDxGPUrh3DMo29Lstif7j",
			Value:     10e8,
			Height:    23505,
			Tx:        "bf41a5352b6d59a401cd946432117b25fd5fc43186aef5cbbe3170c40050d104",
			N:         1,
			Processed: false,
		},
	}

	processedDeposit := Deposit{
		Address:   "1GH9ukgyetEJoWQFwUUeLcWQ8UgVgipLKb",
		Value:     100e8,
		Height:    23517,
		Tx:        "d61be86942d69dc7ba6d49c817957ecd0918798f030c73739206e6f48fe2a7c5",
		N:         1,
		Processed: true,
	}

	err := scr.store.(*Store).db.Update(func(tx *bolt.Tx) error {
		for _, d := range unprocessedDeposits {
			if err := scr.store.(*Store).pushDepositTx(tx, d); err != nil {
				require.NoError(t, err)
				return err
			}
		}

		// Add a processed deposit to make sure that processed deposits are filtered
		return scr.store.(*Store).pushDepositTx(tx, processedDeposit)
	})
	require.NoError(t, err)

	// Don't add any watch addresses,
	// only process the unprocessed deposits from the backlog
	done := make(chan struct{})
	go func() {
		defer close(done)
		var dvs []DepositNote
		for dv := range scr.GetDeposit() {
			dvs = append(dvs, dv)
			dv.ErrC <- nil
		}

		require.Len(t, dvs, len(unprocessedDeposits))

		err := scr.store.(*Store).db.View(func(tx *bolt.Tx) error {
			for _, dv := range dvs {
				var d Deposit
				err := dbutil.GetBucketObject(tx, depositBkt, dv.TxN(), &d)
				require.NoError(t, err)
				require.True(t, d.Processed)
				if err != nil {
					return err
				}
			}

			return nil
		})
		require.NoError(t, err)
	}()

	time.AfterFunc(time.Millisecond*300, func() {
		scr.Shutdown()
	})

	err = scr.Run()
	require.NoError(t, err)
	<-done
}

func TestScannerScanBlockFailureRetry(t *testing.T) {
	// TODO
	// Test that when scanBlock() fails, it logs "Scan block failed"
	// and retries scan of the same block after ScanPeriod elapses.
}
