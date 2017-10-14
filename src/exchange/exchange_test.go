package exchange

import (
	"sync"
	"testing"

	"time"

	"github.com/boltdb/bolt"
	"github.com/skycoin/teller/src/scanner"
	"github.com/skycoin/teller/src/sender"
	"github.com/skycoin/teller/src/util/testutil"
	"github.com/stretchr/testify/require"
)

type dummySender struct {
	sync.RWMutex
	txid           string
	sendErr        error
	confirmErr     error
	txidConfirmMap map[string]bool
}

func newDummySender() *dummySender {
	return &dummySender{
		txidConfirmMap: make(map[string]bool),
	}
}

func (send *dummySender) Send(destAddr string, coins uint64) *sender.SendResponse {
	req := sender.SendRequest{
		Coins:   coins,
		Address: destAddr,
		RspC:    make(chan *sender.SendResponse, 1),
	}

	if send.sendErr != nil {
		return &sender.SendResponse{
			Err: send.sendErr,
			Req: req,
		}
	}

	return &sender.SendResponse{
		Txid: send.txid,
		Req:  req,
	}
}

func (send *dummySender) IsTxConfirmed(txid string) *sender.ConfirmResponse {
	send.RLock()
	defer send.RUnlock()

	req := sender.ConfirmRequest{
		Txid: txid,
	}

	if send.confirmErr != nil {
		return &sender.ConfirmResponse{
			Err: send.confirmErr,
			Req: req,
		}
	}

	confirmed := send.txidConfirmMap[txid]
	return &sender.ConfirmResponse{
		Confirmed: confirmed,
		Req:       req,
	}
}

func (send *dummySender) setTxConfirmed(txid string) {
	send.Lock()
	defer send.Unlock()

	send.txidConfirmMap[txid] = true
}

type dummyScanner struct {
	dvC   chan scanner.DepositNote
	addrs []string
}

func newDummyScanner() *dummyScanner {
	return &dummyScanner{
		dvC: make(chan scanner.DepositNote, 10),
	}
}

func (scan *dummyScanner) AddScanAddress(addr string) error {
	scan.addrs = append(scan.addrs, addr)
	return nil
}

func (scan *dummyScanner) GetDeposit() <-chan scanner.DepositNote {
	return scan.dvC
}

func (scan *dummyScanner) GetScanAddresses() ([]string, error) {
	return []string{}, nil
}

func (scan *dummyScanner) addDeposit(d scanner.DepositNote) {
	scan.dvC <- d
}

func (scan *dummyScanner) stop() {
	close(scan.dvC)
}

const testSkyBtcRate = 100

func newTestExchange(t *testing.T, db *bolt.DB) *Exchange {
	e, err := NewExchange(testutil.NewLogger(t), db, newDummyScanner(), newDummySender(), Config{
		Rate: testSkyBtcRate,
		TxConfirmationCheckWait: time.Millisecond * 100,
	})
	require.NoError(t, err)
	return e
}

func testExchange(t *testing.T, f func(t *testing.T, e *Exchange)) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	e := newTestExchange(t, db)

	done := make(chan struct{})
	go func() {
		err := e.Run()
		require.NoError(t, err)
		close(done)
	}()

	f(t, e)

	e.Shutdown()
	<-done
}

func TestRunShutdown(t *testing.T) {
	// Tests a simple start and stop, with no scanner activity
	testExchange(t, func(t *testing.T, e *Exchange) {})
}

func TestRunScannerClosed(t *testing.T) {
	// Tests that there is no problem when the scanner closes
	testExchange(t, func(t *testing.T, e *Exchange) {
		e.scanner.(*dummyScanner).stop()
	})
}

func TestRunSend(t *testing.T) {
	testExchange(t, testRunSend)
}

func testRunSend(t *testing.T, e *Exchange) {
	skyAddr := "foo-sky-addr"
	btcAddr := "foo-btc-addr"
	err := e.store.BindAddress(skyAddr, btcAddr)
	require.NoError(t, err)

	txid := "foo-sky-txid"
	e.sender.(*dummySender).txid = txid

	dn := scanner.DepositNote{
		Deposit: scanner.Deposit{
			Address: btcAddr,
			Value:   1e8,
			Height:  20,
			Tx:      "foo-tx",
			N:       2,
		},
		ErrC: make(chan error, 1),
	}
	e.scanner.(*dummyScanner).addDeposit(dn)

	// First loop calls saveIncomingDeposit
	// nil is written to ErrC after this method finishes
	err = <-dn.ErrC
	require.NoError(t, err)

	// Second loop calls processWaitSendDeposit
	// It sends the coins, then confirms them

	timeout := time.Second * 3
	checkWaitTime := time.Millisecond * 300

	// Periodically check the database until we observe the sent deposit
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-time.After(checkWaitTime):
				di, err := e.store.GetDepositInfo(dn.Deposit.TxN())
				require.NoError(t, err)
				if di.Status != StatusWaitConfirm {
					continue
				}

				require.NotEmpty(t, di.UpdatedAt)

				expectedDeposit := DepositInfo{
					Seq:        1,
					UpdatedAt:  di.UpdatedAt,
					Status:     StatusWaitConfirm,
					SkyAddress: skyAddr,
					BtcAddress: dn.Deposit.Address,
					BtcTx:      dn.Deposit.TxN(),
					Txid:       txid,
					SkySent:    100e6,
					SkyBtcRate: testSkyBtcRate,
					Deposit:    dn.Deposit,
				}

				require.Equal(t, expectedDeposit, di)
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatal("Waiting for sent deposit timed out")
	}

	// Mark the deposit as confirmed
	e.sender.(*dummySender).setTxConfirmed(txid)

	// Periodically check the database until we observe the confirmed deposit
	done = make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-time.After(checkWaitTime):
				di, err := e.store.GetDepositInfo(dn.Deposit.TxN())
				require.NoError(t, err)
				if di.Status != StatusDone {
					continue
				}

				require.NotEmpty(t, di.UpdatedAt)

				expectedDeposit := DepositInfo{
					Seq:        1,
					UpdatedAt:  di.UpdatedAt,
					Status:     StatusDone,
					SkyAddress: skyAddr,
					BtcAddress: dn.Deposit.Address,
					BtcTx:      dn.Deposit.TxN(),
					Txid:       txid,
					SkySent:    100e6,
					SkyBtcRate: testSkyBtcRate,
					Deposit:    dn.Deposit,
				}

				require.Equal(t, expectedDeposit, di)
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatal("Waiting for confirmed deposit timed out")
	}
}

// TODO -- test various failure cases
