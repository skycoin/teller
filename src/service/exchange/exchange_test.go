package exchange

import (
	"fmt"
	"testing"

	"time"

	"github.com/skycoin/teller/src/logger"
	"github.com/skycoin/teller/src/service/scanner"
	"github.com/skycoin/teller/src/service/sender"
	"github.com/stretchr/testify/require"
)

type dummySender struct {
	txid      string
	err       error
	sleepTime time.Duration
	sent      struct {
		Address string
		Value   int64
	}
	closed         bool
	txidConfirmMap map[string]bool
}

func (send *dummySender) Send(destAddr string, coins int64, opt *sender.SendOption) (string, error) {
	time.Sleep(send.sleepTime)

	if send.err != nil && send.err != sender.ErrServiceClosed {
		return "", send.err
	}

	send.sent.Address = destAddr
	send.sent.Value = coins
	return send.txid, send.err
}

func (send *dummySender) SendAsync(destAddr string,
	coins int64,
	opt *sender.SendOption) (<-chan interface{}, error) {
	rspC := make(chan interface{}, 1)
	if send.err != nil {
		return rspC, send.err
	}

	stC := make(chan sender.SendStatus, 2)
	time.AfterFunc(100*time.Millisecond, func() {
		send.sent.Address = destAddr
		send.sent.Value = coins
		rspC <- sender.Response{
			StatusC: stC,
			Txid:    send.txid,
		}
		stC <- sender.Sent
	})

	time.AfterFunc(send.sleepTime, func() {
		stC <- sender.TxConfirmed
	})

	return rspC, nil
}

func (send *dummySender) IsClosed() bool {
	return send.closed
}

func (send *dummySender) IsTxConfirmed(txid string) bool {
	return send.txidConfirmMap[txid]
}

type dummyScanner struct {
	dvC         chan scanner.DepositNote
	addrs       []string
	notifyC     chan struct{}
	notifyAfter time.Duration
	closed      bool
}

func (scan *dummyScanner) AddDepositAddress(addr string) error {
	scan.addrs = append(scan.addrs, addr)
	return nil
}

func (scan *dummyScanner) GetDepositValue() <-chan scanner.DepositNote {
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

func (scan *dummyScanner) GetDepositAddresses() []string {
	return []string{}
}

func TestRunExchangeService(t *testing.T) {

	var testCases = []struct {
		name        string
		initDpis    []DepositInfo
		bindBtcAddr string
		bindSkyAddr string
		dpAddr      string
		dpValue     float64

		sendSleepTime  time.Duration
		sendReturnTxid string
		sendErr        error

		sendServClosed bool

		dvC           chan scanner.DepositValue
		scanServClose bool
		notifyAfter   time.Duration
		txmap         map[string]bool

		putDVTime    time.Duration
		writeToDBOk  bool
		expectStatus Status
	}{
		{
			name:           "ok",
			initDpis:       []DepositInfo{},
			bindBtcAddr:    "btcaddr",
			bindSkyAddr:    "skyaddr",
			dpAddr:         "btcaddr",
			dpValue:        0.002,
			sendSleepTime:  time.Second * 1,
			sendReturnTxid: "1111",
			sendErr:        nil,

			dvC:         make(chan scanner.DepositValue, 1),
			notifyAfter: 3 * time.Second,
			txmap:       make(map[string]bool),

			putDVTime:    1 * time.Second,
			writeToDBOk:  true,
			expectStatus: StatusDone,
		},
		{
			name:           "deposit_addr_not_exist",
			initDpis:       []DepositInfo{},
			bindBtcAddr:    "btcaddr",
			bindSkyAddr:    "skyaddr",
			dpAddr:         "btcaddr1",
			dpValue:        0.002,
			sendSleepTime:  time.Second * 1,
			sendReturnTxid: "1111",
			sendErr:        nil,
			dvC:            make(chan scanner.DepositValue, 1),
			notifyAfter:    3 * time.Second,
			txmap:          make(map[string]bool),
			putDVTime:      1 * time.Second,
			writeToDBOk:    false,
			expectStatus:   StatusWaitDeposit,
		},
		{
			name: "deposit_status_above_waiting_btc_deposit",
			initDpis: []DepositInfo{
				{BtcAddress: "btcaddr", SkyAddress: "skyaddr", Status: StatusDone},
			},
			bindBtcAddr:    "btcaddr",
			bindSkyAddr:    "skyaddr",
			dpAddr:         "btcaddr",
			dpValue:        0.002,
			sendSleepTime:  time.Second * 1,
			sendReturnTxid: "1111",
			sendErr:        nil,
			dvC:            make(chan scanner.DepositValue, 1),
			notifyAfter:    3 * time.Second,
			txmap:          make(map[string]bool),
			putDVTime:      1 * time.Second,
			writeToDBOk:    true,
			expectStatus:   StatusDone,
		},
		{
			name:           "send_service_closed",
			initDpis:       []DepositInfo{},
			bindBtcAddr:    "btcaddr",
			bindSkyAddr:    "skyaddr",
			dpAddr:         "btcaddr",
			dpValue:        0.002,
			sendSleepTime:  time.Second * 1,
			sendReturnTxid: "1111",
			sendErr:        sender.ErrServiceClosed,
			sendServClosed: true,
			dvC:            make(chan scanner.DepositValue, 1),
			notifyAfter:    3 * time.Second,
			txmap:          make(map[string]bool),
			putDVTime:      1 * time.Second,
			writeToDBOk:    true,
			expectStatus:   StatusWaitSend,
		},
		{
			name:           "send_failed",
			initDpis:       []DepositInfo{},
			bindBtcAddr:    "btcaddr",
			bindSkyAddr:    "skyaddr",
			dpAddr:         "btcaddr",
			dpValue:        0.002,
			sendSleepTime:  time.Second * 3,
			sendReturnTxid: "",
			sendErr:        fmt.Errorf("send skycoin failed"),
			dvC:            make(chan scanner.DepositValue, 1),
			notifyAfter:    3 * time.Second,
			txmap:          make(map[string]bool),
			putDVTime:      1 * time.Second,
			writeToDBOk:    true,
			expectStatus:   StatusWaitSend,
		},
		{
			name:           "scan_service_closed",
			initDpis:       []DepositInfo{},
			bindBtcAddr:    "btcaddr",
			bindSkyAddr:    "skyaddr",
			dpAddr:         "btcaddr",
			dpValue:        0.002,
			sendSleepTime:  time.Second * 3,
			sendReturnTxid: "",
			sendErr:        fmt.Errorf("send skycoin failed"),
			dvC:            make(chan scanner.DepositValue, 1),
			notifyAfter:    3 * time.Second,
			txmap:          make(map[string]bool),
			scanServClose:  true,
			putDVTime:      1 * time.Second,
			writeToDBOk:    true,
			expectStatus:   StatusWaitSend,
		},
		{
			name: "has_unconfrimed_tx",
			initDpis: []DepositInfo{
				{BtcAddress: "btcaddr", SkyAddress: "skyaddr", Txid: "t1", Status: StatusWaitConfirm},
			},
			bindBtcAddr:    "btcaddr",
			bindSkyAddr:    "skyaddr",
			dpAddr:         "btcaddr",
			dpValue:        0.002,
			sendSleepTime:  time.Second * 3,
			sendReturnTxid: "",
			sendErr:        fmt.Errorf("send skycoin failed"),
			dvC:            make(chan scanner.DepositValue, 1),
			notifyAfter:    3 * time.Second,
			txmap:          map[string]bool{"t1": true},
			scanServClose:  true,
			putDVTime:      1 * time.Second,
			writeToDBOk:    true,
			expectStatus:   StatusWaitSend,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, shutdown := setupDB(t)
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
			var service *Service

			require.NotPanics(t, func() {
				service = NewService(Config{
					Rate: 500,
				}, db, logger.NewLogger("", true), scan, send)

				// init the deposit infos
				for _, dpi := range tc.initDpis {
					service.store.AddDepositInfo(dpi)
				}
			})

			go service.Run()

			excli := NewClient(service)
			if len(tc.initDpis) == 0 {
				require.Nil(t, excli.BindAddress(tc.bindBtcAddr, tc.bindSkyAddr))
			}

			// fake deposit value
			time.AfterFunc(tc.putDVTime, func() {
				if scan.closed {
					close(dvC)
					return
				}
				dvC <- scanner.DepositNote{
					DepositValue: scanner.DepositValue{
						Address: tc.dpAddr,
						Value:   tc.dpValue,
					},
					AckC: make(chan struct{}, 1),
				}
			})

			<-scan.notifyC

			if scan.closed {
				return
			}

			// check the info
			dpi, ok := service.store.GetDepositInfo(tc.dpAddr)
			require.Equal(t, tc.writeToDBOk, ok)
			if ok {
				require.Equal(t, tc.expectStatus, dpi.Status)

				if len(tc.initDpis) == 0 && tc.sendErr == nil {
					require.Equal(t, tc.bindSkyAddr, send.sent.Address)
					require.Equal(t, int64(tc.dpValue*500), send.sent.Value)
				}
			}

			service.Shutdown()
		})
	}

}
