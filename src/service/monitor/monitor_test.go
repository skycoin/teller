package monitor

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/util/logger"
	"github.com/skycoin/teller/src/service/exchange"
	"github.com/stretchr/testify/require"
)

type dummyBtcAddrMgr struct {
	Num uint64
}

func (db *dummyBtcAddrMgr) RestNum() uint64 {
	return db.Num
}

type dummyDepositStatusGetter struct {
	dpis []exchange.DepositInfo
}

func (dps dummyDepositStatusGetter) GetDepositStatusDetail(flt exchange.DepositFilter) ([]daemon.DepositStatusDetail, error) {
	var ds []daemon.DepositStatusDetail
	for _, dpi := range dps.dpis {
		if flt(dpi) {
			ds = append(ds, daemon.DepositStatusDetail{
				Seq:        dpi.Seq,
				BtcAddress: dpi.BtcAddress,
				SkyAddress: dpi.SkyAddress,
				Status:     dpi.Status.String(),
				UpdateAt:   dpi.UpdatedAt,
				Txid:       dpi.Txid,
			})
		}
	}
	return ds, nil
}

type dummyScanAddrs struct {
	addrs []string
}

func (ds dummyScanAddrs) GetScanAddresses() ([]string, error) {
	return []string{}, nil
}

func TestRunMonitor(t *testing.T) {
	dpis := []exchange.DepositInfo{
		{
			BtcAddress: "b1",
			SkyAddress: "s1",
			Status:     exchange.StatusWaitDeposit,
		},
		{
			BtcAddress: "b2",
			SkyAddress: "s2",
			Status:     exchange.StatusWaitSend,
		},
		{
			BtcAddress: "b3",
			SkyAddress: "s3",
			Status:     exchange.StatusWaitConfirm,
		},
		{
			BtcAddress: "b4",
			SkyAddress: "s4",
			Status:     exchange.StatusDone,
		},
		{
			BtcAddress: "b5",
			SkyAddress: "s6",
			Status:     exchange.StatusDone,
		},
	}

	dummyDps := dummyDepositStatusGetter{dpis: dpis}

	cfg := Config{
		"localhost:7908",
	}

	m := New(cfg, logger.NewLogger("", true), &dummyBtcAddrMgr{10}, &dummyDps, &dummyScanAddrs{})

	time.AfterFunc(1*time.Second, func() {
		rsp, err := http.Get(fmt.Sprintf("http://localhost:7908/api/address"))
		require.Nil(t, err)
		require.Equal(t, 200, rsp.StatusCode)
		var addrUsage addressUsage
		err = json.NewDecoder(rsp.Body).Decode(&addrUsage)
		require.Nil(t, err)
		require.Equal(t, uint64(10), addrUsage.RestAddrNum)
		rsp.Body.Close()

		var tt = []struct {
			name        string
			status      string
			expectCode  int //
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
				"get unknow status",
				"invalid",
				http.StatusBadRequest,
				nil,
			},
		}

		for _, tc := range tt {
			t.Run(tc.name, func(t *testing.T) {
				rsp, err := http.Get(fmt.Sprintf("http://localhost:7908/api/deposit_status?status=%s", tc.status))
				require.Nil(t, err)
				defer rsp.Body.Close()
				require.Equal(t, tc.expectCode, rsp.StatusCode)
				if rsp.StatusCode == 200 {
					var st []daemon.DepositStatusDetail
					require.Nil(t, json.NewDecoder(rsp.Body).Decode(&st))
					dss := make([]exchange.DepositInfo, 0, len(st))
					for _, s := range st {
						dss = append(dss, exchange.DepositInfo{
							Seq:        s.Seq,
							UpdatedAt:  s.UpdateAt,
							Status:     exchange.NewStatusFromStr(s.Status),
							BtcAddress: s.BtcAddress,
							SkyAddress: s.SkyAddress,
							Txid:       s.Txid,
						})
					}
					require.Equal(t, tc.expectValue, dss)
				}
			})
		}

		m.Shutdown()
	})

	if err := m.Run(); err != nil {
		return
	}
}
