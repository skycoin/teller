package exchange

import (
	"testing"

	"time"

	"fmt"

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

type dummyScanner struct {
	dvC         chan scanner.DepositValue
	addrs       []string
	notifyC     chan struct{}
	notifyAfter time.Duration
}

func (scan *dummyScanner) AddDepositAddress(addr string) error {
	scan.addrs = append(scan.addrs, addr)
	return nil
}

func (scan *dummyScanner) GetDepositValue() <-chan scanner.DepositValue {
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

func TestRunExchangeService(t *testing.T) {

	var testCases = []struct {
		name        string
		initDpis    []depositInfo
		bindBtcAddr string
		bindSkyAddr string
		dpAddr      string
		dpValue     float64

		sendSleepTime  time.Duration
		sendReturnTxid string
		sendErr        error

		dvC         chan scanner.DepositValue
		notifyAfter time.Duration

		putDVTime    time.Duration
		writeToDBOk  bool
		expectStatus status
	}{
		{
			name:           "ok",
			initDpis:       []depositInfo{},
			bindBtcAddr:    "btcaddr",
			bindSkyAddr:    "skyaddr",
			dpAddr:         "btcaddr",
			dpValue:        0.002,
			sendSleepTime:  time.Second * 1,
			sendReturnTxid: "1111",
			sendErr:        nil,

			dvC:          make(chan scanner.DepositValue, 1),
			notifyAfter:  3 * time.Second,
			putDVTime:    1 * time.Second,
			writeToDBOk:  true,
			expectStatus: statusDone,
		},
		{
			name:           "deposit_addr_not_exist",
			initDpis:       []depositInfo{},
			bindBtcAddr:    "btcaddr",
			bindSkyAddr:    "skyaddr",
			dpAddr:         "btcaddr1",
			dpValue:        0.002,
			sendSleepTime:  time.Second * 1,
			sendReturnTxid: "1111",
			sendErr:        nil,
			dvC:            make(chan scanner.DepositValue, 1),
			notifyAfter:    3 * time.Second,
			putDVTime:      1 * time.Second,
			writeToDBOk:    false,
			expectStatus:   statusWaitBtcDeposit,
		},
		{
			name: "deposit_status_above_waiting_btc_deposit",
			initDpis: []depositInfo{
				{BtcAddress: "btcaddr", SkyAddress: "skyaddr", Status: statusDone},
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
			putDVTime:      1 * time.Second,
			writeToDBOk:    true,
			expectStatus:   statusDone,
		},
		{
			name:           "send_service_closed",
			initDpis:       []depositInfo{},
			bindBtcAddr:    "btcaddr",
			bindSkyAddr:    "skyaddr",
			dpAddr:         "btcaddr",
			dpValue:        0.002,
			sendSleepTime:  time.Second * 1,
			sendReturnTxid: "1111",
			sendErr:        sender.ErrServiceClosed,
			dvC:            make(chan scanner.DepositValue, 1),
			notifyAfter:    3 * time.Second,
			putDVTime:      1 * time.Second,
			writeToDBOk:    true,
			expectStatus:   statusDone,
		},
		{
			name:           "send_failed",
			initDpis:       []depositInfo{},
			bindBtcAddr:    "btcaddr",
			bindSkyAddr:    "skyaddr",
			dpAddr:         "btcaddr",
			dpValue:        0.002,
			sendSleepTime:  time.Second * 3,
			sendReturnTxid: "",
			sendErr:        fmt.Errorf("send skycoin failed"),
			dvC:            make(chan scanner.DepositValue, 1),
			notifyAfter:    3 * time.Second,
			putDVTime:      1 * time.Second,
			writeToDBOk:    true,
			expectStatus:   statusWaitSkySend,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db, shutdown := setupDB(t)
			defer shutdown()

			send := &dummySender{
				sleepTime: tc.sendSleepTime,
				txid:      tc.sendReturnTxid,
				err:       tc.sendErr,
			}

			dvC := make(chan scanner.DepositValue)
			scan := &dummyScanner{
				dvC:         dvC,
				notifyC:     make(chan struct{}, 1),
				notifyAfter: tc.notifyAfter,
			}
			var service *Service

			require.NotPanics(t, func() {
				service = NewService(Config{
					DB:   db,
					Log:  logger.NewLogger("", true),
					Rate: 500,
				}, scan, send)

				// init the deposit infos
				for _, dpi := range tc.initDpis {
					service.store.AddDepositInfo(dpi)
				}
			})

			go service.Run()

			excli := NewExchange(service)
			if len(tc.initDpis) == 0 {
				require.Nil(t, excli.BindAddress(tc.bindBtcAddr, tc.bindSkyAddr))
			}

			// fake deposit value
			time.AfterFunc(tc.putDVTime, func() {
				dvC <- scanner.DepositValue{Address: tc.dpAddr, Value: tc.dpValue}
			})

			<-scan.notifyC

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
