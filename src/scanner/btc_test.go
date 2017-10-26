package scanner

import (
	"encoding/json"
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

type dummyBtcrpcclient struct {
	db        *bolt.DB
	bestBlock LastScanBlock
	lastBlock LastScanBlock
}

func openDummyBtcDB(t *testing.T) *bolt.DB {
	// Blocks 235205 through 235212 are stored in this DB
	db, err := bolt.Open("./btc.db", 0600, nil)
	require.NoError(t, err)
	return db
}

func newDummyBtcrpcclient(db *bolt.DB) *dummyBtcrpcclient {
	return &dummyBtcrpcclient{db: db}
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

func (dbc *dummyBtcrpcclient) GetLastScanBlock() (*chainhash.Hash, int32, error) {
	hash, err := chainhash.NewHashFromStr(dbc.lastBlock.Hash)
	if err != nil {
		return nil, 0, err
	}

	return hash, int32(dbc.lastBlock.Height), nil
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

func TestScannerRun(t *testing.T) {
	// Tests that the scanner will scan multiple blocks sequentially, finding
	// all relevant deposits and adding them to the depositC channel.
	// All deposits on the depositC channel will be successfully processed
	// by the channel reader, and the scanner will mark these deposits as
	// "processed".
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	btcDB := openDummyBtcDB(t)
	defer btcDB.Close()

	// Blocks 235205 through 235212 are stored in btc.db
	// Refer to https://blockchain.info or another explorer to see the block data
	rpcclient := newDummyBtcrpcclient(btcDB)
	rpcclient.lastBlock = LastScanBlock{
		Hash:   "000000000000018d8ece83a004c5a919210d67798d13aa901c4d07f8bf87b719",
		Height: 235205,
	}

	rpcclient.bestBlock = LastScanBlock{
		Hash:   "000000000000014e5217c81d6228a9274395a8bee3eb87277dd9e4315ee0f439",
		Height: 235207,
	}

	store, err := NewStore(log, db)
	require.NoError(t, err)

	cfg := Config{
		ScanPeriod:        time.Millisecond * 10,
		DepositBufferSize: 5,
	}
	scr, err := NewBTCScanner(log, store, rpcclient, cfg)
	require.NoError(t, err)

	err = scr.store.(*Store).db.Update(func(tx *bolt.Tx) error {
		return scr.store.(*Store).setLastScanBlockTx(tx, rpcclient.lastBlock)
	})
	require.NoError(t, err)

	nDeposits := 0

	// This address has 1 deposit, in block 235207
	err = scr.AddScanAddress("1LcEkgX8DCrQczLMVh9LDTRnkdVV2oun3A")
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
	require.True(t, cfg.DepositBufferSize < nDeposits)

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
		err := db.View(func(tx *bolt.Tx) error {
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

	time.AfterFunc(time.Second*3, func() {
		scr.Shutdown()
	})

	err = scr.Run()
	require.NoError(t, err)
	<-done
}

func TestLoadUnprocessedDeposits(t *testing.T) {
	// TODO
	// Test that pending unprocessed deposits from the db are loaded when
	// then scanner starts.
}

func TestScanBlockFailureRetry(t *testing.T) {
	// TODO
	// Test that when scanBlock() fails, it logs "Scan block failed"
	// and retries scan of the same block after ScanPeriod elapses.
}

func TestProcessDepositError(t *testing.T) {
	// TODO
	// Test that when processDeposit() fails, the deposit is NOT marked as
	// processed, and that a warning message is logged.
}
