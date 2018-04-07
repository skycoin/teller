package monitor

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skycoin/teller/src/addrs"
	"github.com/skycoin/teller/src/config"
	"github.com/skycoin/teller/src/exchange"
	"github.com/skycoin/teller/src/util/testutil"
	"github.com/boltdb/bolt"
)

type dummyBtcAddrMgr struct {
	Num uint64
}
type dummyEthAddrMgr struct {
	Num uint64
}
type dummySkyAddrMgr struct {
	Num uint64
}

func (db *dummyBtcAddrMgr) NewAddress() (string, error) {
	return "", errors.New("not implemented")
}

func (db *dummyBtcAddrMgr) Remaining() uint64 {
	return db.Num
}

func (db *dummyEthAddrMgr) NewAddress() (string, error) {
	return "", errors.New("not implemented")
}

func (db *dummyEthAddrMgr) Remaining() uint64 {
	return db.Num
}

func (db *dummySkyAddrMgr) NewAddress() (string, error) {
	return "", errors.New("not implemented")
}

func (db *dummySkyAddrMgr) Remaining() uint64 {
	return db.Num
}

type dummyDepositStatusGetter struct {
	dpis []exchange.DepositInfo
}

func (dps dummyDepositStatusGetter) GetDeposits(flt exchange.DepositFilter) ([]exchange.DepositInfo, error) {
	var ds []exchange.DepositInfo
	for _, dpi := range dps.dpis {
		if flt(dpi) {
			ds = append(ds, dpi)
		}
	}
	return ds, nil
}

func (dps dummyDepositStatusGetter) GetDepositStats() (*exchange.DepositStats, error) {
	received := make(map[string]int64)
	var sent int64

	for _, dpi := range dps.dpis {
		received[dpi.CoinType] += dpi.DepositValue
		sent += int64(dpi.SkySent)
	}

	return &exchange.DepositStats{
		Received: received,
		Sent:     sent,
	}, nil
}

func (dps dummyDepositStatusGetter) ErroredDeposits() ([]exchange.DepositInfo, error) {
	return nil, nil
}

type dummyScanAddrs struct {
	// addrs []string
}

func (ds dummyScanAddrs) GetScanAddresses(string) ([]string, error) {
	return []string{}, nil
}

func TestRunMonitor(t *testing.T) {
	dpis := []exchange.DepositInfo{
		{
			DepositAddress: "b1",
			SkyAddress:     "s1",
			Status:         exchange.StatusWaitDeposit,
		},
		{
			DepositAddress: "b2",
			SkyAddress:     "s2",
			Status:         exchange.StatusWaitSend,
		},
		{
			DepositAddress: "b3",
			SkyAddress:     "s3",
			Status:         exchange.StatusWaitConfirm,
		},
		{
			DepositAddress: "b4",
			SkyAddress:     "s4",
			Status:         exchange.StatusDone,
		},
		{
			DepositAddress: "b5",
			SkyAddress:     "s6",
			Status:         exchange.StatusDone,
		},
	}

	dummyDps := dummyDepositStatusGetter{dpis: dpis}

	cfg := config.Config{
		AdminPanel: config.AdminPanel{
			Host: "localhost:7908",
		},
	}

	log, _ := testutil.NewLogger(t)

	addrMgr := addrs.NewAddrManager()

	err := addrMgr.PushGenerator(&dummyBtcAddrMgr{10}, config.CoinTypeBTC)
	require.NoError(t, err)
	err = addrMgr.PushGenerator(&dummyEthAddrMgr{11}, config.CoinTypeETH)
	require.NoError(t, err)
	err = addrMgr.PushGenerator(&dummySkyAddrMgr{12}, config.CoinTypeSKY)
	require.NoError(t, err)

	m := New(log, cfg, addrMgr, &dummyDps, &dummyScanAddrs{}, &bolt.DB{})

	done := make(chan struct{})
	go func() {
		defer close(done)
		err := m.Run()
		require.NoError(t, err)
	}()

	// Wait for the server to start
	time.Sleep(time.Second)

	rsp, err := http.Get(fmt.Sprintf("http://localhost:7908/api/deposit-addresses"))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, rsp.StatusCode)

	var addrUsage map[string]depositAddressStats
	err = json.NewDecoder(rsp.Body).Decode(&addrUsage)
	require.NoError(t, err)
	require.Equal(t, uint64(10), addrUsage[config.CoinTypeBTC].RemainingAddresses)
	require.Equal(t, uint64(11), addrUsage[config.CoinTypeETH].RemainingAddresses)
	require.Equal(t, uint64(12), addrUsage[config.CoinTypeSKY].RemainingAddresses)
	testutil.CheckError(t, rsp.Body.Close)

	var tt = []struct {
		name        string
		status      string
		expectCode  int
		expectValue []exchange.DepositInfo
	}{
		{
			"get deposit that are in waiting_deposit status",
			"waiting_deposit",
			http.StatusOK,
			dpis[:1],
		},
		{
			"get deposit that are in waiting_send status",
			"waiting_send",
			http.StatusOK,
			dpis[1:2],
		},
		{
			"get deposit that are in waiting_confirm status",
			"waiting_confirm",
			http.StatusOK,
			dpis[2:3],
		},
		{
			"get deposit that are in waiting_done status",
			"done",
			http.StatusOK,
			dpis[3:5],
		},
		{
			"get unknown status",
			"invalid",
			http.StatusBadRequest,
			nil,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			rsp, err := http.Get(fmt.Sprintf("http://localhost:7908/api/deposits?status=%s", tc.status))
			require.NoError(t, err)
			defer testutil.CheckError(t, rsp.Body.Close)
			require.Equal(t, tc.expectCode, rsp.StatusCode)

			if rsp.StatusCode == http.StatusOK {
				var st depositsByStatusResponse
				err := json.NewDecoder(rsp.Body).Decode(&st)
				require.NoError(t, err)

				dss := make([]exchange.DepositInfo, 0, len(st.Deposits))
				for _, s := range st.Deposits {
					dss = append(dss, exchange.DepositInfo{
						Seq:            s.Seq,
						UpdatedAt:      s.UpdatedAt,
						Status:         s.Status,
						DepositAddress: s.DepositAddress,
						SkyAddress:     s.SkyAddress,
						Txid:           s.Txid,
					})
				}
				require.Equal(t, tc.expectValue, dss)
			}
		})
	}

	m.Shutdown()
	<-done
}
