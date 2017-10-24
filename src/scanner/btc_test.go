package scanner

import (
	"encoding/json"
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

type blockHashHeight struct {
	Hash   string
	Height int64
}

type dummyBtcrpcclient struct {
	db        *bolt.DB
	bestBlock blockHashHeight
	lastBlock blockHashHeight
}

func openDummyBtcDB(t *testing.T) *bolt.DB {
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
			return nil
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

	// TODO -- this uses real data from test.db, but it is opaque.
	// It is not easy to see what the data should be.
	// Use hardcoded btcjson.GetBlockVerboseResult responses instead
	// They can have fake data.
	rpcclient := newDummyBtcrpcclient(btcDB)
	rpcclient.lastBlock = blockHashHeight{
		Hash:   "00000000000001749cf1a15c5af397a04a18d09e9bc902b6ce70f64bc19acc98",
		Height: 235203,
	}

	rpcclient.bestBlock = blockHashHeight{
		Hash:   "000000000000018d8ece83a004c5a919210d67798d13aa901c4d07f8bf87b719",
		Height: 235205,
	}

	store, err := NewStore(log, db)
	require.NoError(t, err)

	scr, err := NewBTCScanner(log, store, rpcclient, Config{
		ScanPeriod: time.Millisecond * 100,
	})

	require.NoError(t, err)

	err = scr.AddScanAddress("1ATjE4kwZ5R1ww9SEi4eseYTCenVgaxPWu")
	require.NoError(t, err)
	err = scr.AddScanAddress("1EYQ7Fnct6qu1f3WpTSib1UhDhxkrww1WH")
	require.NoError(t, err)
	err = scr.AddScanAddress("1LEkderht5M5yWj82M87bEd4XDBsczLkp9")
	require.NoError(t, err)

	time.AfterFunc(time.Second, func() {
		var dvs []DepositNote
		for dv := range scr.GetDeposit() {
			dvs = append(dvs, dv)
			time.Sleep(100 * time.Millisecond)
			dv.ErrC <- nil
		}
		require.Len(t, dvs, 127)

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
	})

	time.AfterFunc(time.Second*2, func() {
		scr.Shutdown()
	})

	err = scr.Run()
	require.NoError(t, err)
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
