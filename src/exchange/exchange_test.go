package exchange

import (
	"errors"
	"fmt"
	"testing"

	"time"

	"github.com/stretchr/testify/require"

	"github.com/skycoin/teller/src/scanner"
	"github.com/skycoin/teller/src/sender"
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

func TestRunExchange(t *testing.T) {
	// TODO -- write this test, should cover all the main cases of run()
	// TODO -- if easier, can write separate tests to cover the methods called by run()
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
