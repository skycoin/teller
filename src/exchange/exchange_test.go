package exchange

import (
	"errors"
	"fmt"
	"testing"

	"time"

	"github.com/stretchr/testify/require"

	"github.com/skycoin/teller/src/scanner"
	"github.com/skycoin/teller/src/sender"
	"github.com/skycoin/teller/src/util/dbutil"
	"github.com/skycoin/teller/src/util/testutil"
)

type dummySender struct {
	txid      string
	err       error
	sleepTime time.Duration
	sent      struct {
		Address string
		Value   uint64
	}
	closed         bool
	txidConfirmMap map[string]bool
}

func (send *dummySender) Send(destAddr string, coins uint64) *sender.SendResponse {
	req := sender.SendRequest{
		Coins:   coins,
		Address: destAddr,
		RspC:    make(chan *sender.SendResponse, 2),
	}

	if send.err != nil {
		return &sender.SendResponse{
			Err: send.err,
			Req: req,
		}
	}

	return &sender.SendResponse{
		Txid: send.txid,
		Req:  req,
	}
}

func (send *dummySender) IsTxConfirmed(txid string) *sender.ConfirmResponse {
	req := sender.ConfirmRequest{
		Txid: txid,
	}

	if send.err != nil {
		return &sender.ConfirmResponse{
			Err: send.err,
			Req: req,
		}
	}

	if send.txidConfirmMap[txid] {
		return &sender.ConfirmResponse{
			Confirmed: true,
			Req:       req,
		}
	}

	return &sender.ConfirmResponse{
		Confirmed: false,
		Req:       req,
	}
}

type dummyScanner struct {
	dvC         chan scanner.DepositNote
	addrs       []string
	notifyC     chan struct{}
	notifyAfter time.Duration
	closed      bool
}

func (scan *dummyScanner) AddScanAddress(addr string) error {
	scan.addrs = append(scan.addrs, addr)
	return nil
}

func (scan *dummyScanner) GetDeposit() <-chan scanner.DepositNote {
	defer func() {
		go func() {
			// notify after given duration, so that the test code know
			// it's time do checking
			time.Sleep(scan.notifyAfter)
			scan.notifyC <- struct{}{}
		}()
	}()
	return scan.dvC
}

func (scan *dummyScanner) GetScanAddresses() ([]string, error) {
	return []string{}, nil
}

func TestRunExchangeExchange(t *testing.T) {

	var testCases = []struct {
		name        string
		initDpis    []DepositInfo
		bindBtcAddr string
		bindSkyAddr string
		dpAddr      string
		dpValue     int64
		dpTx        string
		dpN         uint32

		sendSleepTime  time.Duration
		sendReturnTxid string
		sendErr        error

		sendServClosed bool

		dvC           chan scanner.Deposit
		scanServClose bool
		notifyAfter   time.Duration
		txmap         map[string]bool

		putDVTime    time.Duration
		writeToDBOk  bool
		expectStatus Status

		rate int64
		sent uint64
	}{
		{
			name:           "ok",
			initDpis:       []DepositInfo{},
			bindBtcAddr:    "btcaddr",
			bindSkyAddr:    "skyaddr",
			dpAddr:         "btcaddr",
			dpValue:        200000,
			dpTx:           "dptx",
			dpN:            1,
			sendSleepTime:  time.Second * 1,
			sendReturnTxid: "1111",
			sendErr:        nil,
			dvC:            make(chan scanner.Deposit, 1),
			notifyAfter:    3 * time.Second,
			txmap:          make(map[string]bool),
			putDVTime:      1 * time.Second,
			writeToDBOk:    true,
			expectStatus:   StatusDone,
			rate:           500,
			sent:           1000000,
		},

		{
			name:           "deposit_addr_not_exist",
			initDpis:       []DepositInfo{},
			bindBtcAddr:    "btcaddr",
			bindSkyAddr:    "skyaddr",
			dpAddr:         "btcaddr1",
			dpValue:        200000,
			dpTx:           "dptx",
			dpN:            1,
			sendSleepTime:  time.Second * 1,
			sendReturnTxid: "1111",
			sendErr:        nil,
			dvC:            make(chan scanner.Deposit, 1),
			notifyAfter:    3 * time.Second,
			txmap:          make(map[string]bool),
			putDVTime:      1 * time.Second,
			writeToDBOk:    false,
			expectStatus:   StatusWaitDeposit,
			rate:           500,
			sent:           1000000,
		},

		{
			name: "deposit_status_above_waiting_btc_deposit",
			initDpis: []DepositInfo{{
				BtcAddress: "btcaddr",
				SkyAddress: "skyaddr",
				Status:     StatusDone,
				BtcTx:      "dptx:1",
			}},
			bindBtcAddr:    "btcaddr",
			bindSkyAddr:    "skyaddr",
			dpAddr:         "btcaddr",
			dpValue:        200000,
			dpTx:           "dptx",
			dpN:            1,
			sendSleepTime:  time.Second * 1,
			sendReturnTxid: "1111",
			sendErr:        nil,
			dvC:            make(chan scanner.Deposit, 1),
			notifyAfter:    3 * time.Second,
			txmap:          make(map[string]bool),
			putDVTime:      1 * time.Second,
			writeToDBOk:    true,
			expectStatus:   StatusDone,
			rate:           500,
			sent:           1000000,
		},

		{
			name:           "send_service_closed",
			initDpis:       []DepositInfo{},
			bindBtcAddr:    "btcaddr",
			bindSkyAddr:    "skyaddr",
			dpAddr:         "btcaddr",
			dpValue:        200000,
			dpTx:           "dptx",
			dpN:            1,
			sendSleepTime:  time.Second * 1,
			sendReturnTxid: "1111",
			sendErr:        sender.ErrClosed,
			sendServClosed: true,
			dvC:            make(chan scanner.Deposit, 1),
			notifyAfter:    3 * time.Second,
			txmap:          make(map[string]bool),
			putDVTime:      1 * time.Second,
			writeToDBOk:    true,
			expectStatus:   StatusWaitSend,
			rate:           500,
			sent:           1000000,
		},

		{
			name:           "send_failed",
			initDpis:       []DepositInfo{},
			bindBtcAddr:    "btcaddr",
			bindSkyAddr:    "skyaddr",
			dpAddr:         "btcaddr",
			dpValue:        200000,
			dpTx:           "dptx",
			dpN:            1,
			sendSleepTime:  time.Second * 3,
			sendReturnTxid: "",
			sendErr:        fmt.Errorf("send skycoin failed"),
			dvC:            make(chan scanner.Deposit, 1),
			notifyAfter:    3 * time.Second,
			txmap:          make(map[string]bool),
			putDVTime:      1 * time.Second,
			writeToDBOk:    true,
			expectStatus:   StatusWaitSend,
			rate:           500,
			sent:           1000000,
		},

		{
			name:           "scan_exg_closed",
			initDpis:       []DepositInfo{},
			bindBtcAddr:    "btcaddr",
			bindSkyAddr:    "skyaddr",
			dpAddr:         "btcaddr",
			dpValue:        200000,
			dpTx:           "dptx",
			dpN:            1,
			sendSleepTime:  time.Second * 3,
			sendReturnTxid: "",
			sendErr:        fmt.Errorf("send skycoin failed"),
			dvC:            make(chan scanner.Deposit, 1),
			notifyAfter:    3 * time.Second,
			txmap:          make(map[string]bool),
			scanServClose:  true,
			putDVTime:      1 * time.Second,
			writeToDBOk:    true,
			expectStatus:   StatusWaitSend,
			rate:           500,
			sent:           1000000,
		},

		{
			name: "has_unconfirmed_tx",
			initDpis: []DepositInfo{{
				BtcAddress: "btcaddr",
				SkyAddress: "skyaddr",
				Txid:       "t1",
				Status:     StatusWaitConfirm,
				BtcTx:      "dptx:1",
			}},
			bindBtcAddr:    "btcaddr",
			bindSkyAddr:    "skyaddr",
			dpAddr:         "btcaddr",
			dpValue:        200000,
			dpTx:           "dptx",
			dpN:            1,
			sendSleepTime:  time.Second * 3,
			sendReturnTxid: "",
			sendErr:        fmt.Errorf("send skycoin failed"),
			dvC:            make(chan scanner.Deposit, 1),
			notifyAfter:    3 * time.Second,
			txmap:          map[string]bool{"t1": true},
			scanServClose:  true,
			putDVTime:      1 * time.Second,
			writeToDBOk:    true,
			expectStatus:   StatusWaitSend,
			rate:           500,
			sent:           1000000,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, shutdown := testutil.PrepareDB(t)
			defer shutdown()

			send := &dummySender{
				sleepTime:      tc.sendSleepTime,
				txid:           tc.sendReturnTxid,
				err:            tc.sendErr,
				closed:         tc.sendServClosed,
				txidConfirmMap: tc.txmap,
			}

			dvC := make(chan scanner.DepositNote)
			scan := &dummyScanner{
				dvC:         dvC,
				notifyC:     make(chan struct{}, 1),
				notifyAfter: tc.notifyAfter,
				closed:      tc.scanServClose,
			}
			var exg *Exchange

			require.NotPanics(t, func() {
				exg = NewExchange(testutil.NewLogger(t), db, scan, send, Config{
					Rate: tc.rate,
				})

				// init the deposit infos
				for _, dpi := range tc.initDpis {
					err := exg.store.AddDepositInfo(dpi)
					require.NoError(t, err)
				}
			})

			go exg.Run()

			if len(tc.initDpis) == 0 {
				err := exg.BindAddress(tc.bindBtcAddr, tc.bindSkyAddr)
				require.NoError(t, err)
			}

			// fake deposit value
			time.AfterFunc(tc.putDVTime, func() {
				if scan.closed {
					close(dvC)
					return
				}
				dvC <- scanner.DepositNote{
					Deposit: scanner.Deposit{
						Address: tc.dpAddr,
						Value:   tc.dpValue,
						Tx:      tc.dpTx,
						N:       tc.dpN,
					},
					ErrC: make(chan error, 1),
				}
			})

			<-scan.notifyC

			if scan.closed {
				return
			}

			// check the info
			dpTxN := fmt.Sprintf("%s:%d", tc.dpTx, tc.dpN)
			dpi, err := exg.store.GetDepositInfo(dpTxN)

			if tc.writeToDBOk {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				require.IsType(t, dbutil.ObjectNotExistErr{}, err)
			}

			if tc.writeToDBOk {
				require.Equal(t, tc.expectStatus, dpi.Status)

				if len(tc.initDpis) == 0 && tc.sendErr == nil {
					require.Equal(t, tc.bindSkyAddr, send.sent.Address)
					require.Equal(t, tc.sent, send.sent.Value)
				}
			}

			exg.Shutdown()
		})
	}
}

func TestCalculateSkyValue(t *testing.T) {
	cases := []struct {
		satoshis int64
		rate     int64
		result   uint64
		err      error
	}{
		{
			satoshis: -1,
			rate:     1,
			err:      errors.New("negative satoshis or negative skyPerBTC"),
		},

		{
			satoshis: 1,
			rate:     -1,
			err:      errors.New("negative satoshis or negative skyPerBTC"),
		},

		{
			satoshis: 1e8,
			rate:     1,
			result:   1e6,
		},

		{
			satoshis: 1e8,
			rate:     500,
			result:   500e6,
		},

		{
			satoshis: 100e8,
			rate:     500,
			result:   50000e6,
		},

		{
			satoshis: 2e5, // 0.002 BTC
			rate:     500, // 500 SKY/BTC = 1 SKY / 0.002 BTC
			result:   1e6, // 1 SKY
		},
	}

	for _, tc := range cases {
		name := fmt.Sprintf("satoshis=%d rate=%d", tc.satoshis, tc.rate)
		t.Run(name, func(t *testing.T) {
			result, err := calculateSkyValue(tc.satoshis, tc.rate)
			if tc.err == nil {
				require.NoError(t, err)
				require.Equal(t, tc.result, result, "%d != %d", tc.result, result)
			} else {
				require.Error(t, err)
				require.Equal(t, tc.err, err)
				require.Equal(t, uint64(0), result, "%d != 0", result)
			}
		})
	}
}
