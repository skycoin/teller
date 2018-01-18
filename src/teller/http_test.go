package teller

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/skycoin/teller/src/exchange"
	"github.com/skycoin/teller/src/util/testutil"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type fakeExchanger struct {
	mock.Mock
}

func (e *fakeExchanger) BindAddress(skyAddr, depositAddr, coinType string) error {
	args := e.Called(skyAddr, depositAddr, coinType)
	return args.Error(0)
}

func (e *fakeExchanger) GetDepositStatuses(skyAddr string) ([]exchange.DepositStatus, error) {
	args := e.Called(skyAddr)
	return args.Get(0).([]exchange.DepositStatus), args.Error(1)
}

func (e *fakeExchanger) GetDepositStatusDetail(flt exchange.DepositFilter) ([]exchange.DepositStatusDetail, error) {
	args := e.Called(flt)
	return args.Get(0).([]exchange.DepositStatusDetail), args.Error(1)
}

func (e *fakeExchanger) GetBindNum(skyAddr string) (int, error) {
	args := e.Called(skyAddr)
	return args.Int(0), args.Error(1)
}

func (e *fakeExchanger) GetDepositStats() (*exchange.DepositStats, error) {
	args := e.Called()
	return args.Get(0).(*exchange.DepositStats), args.Error(1)
}

func (e *fakeExchanger) Status() error {
	args := e.Called()
	return args.Error(0)
}

func TestExchangeStatusHandler(t *testing.T) {
	tt := []struct {
		name           string
		method         string
		url            string
		status         int
		err            string
		exchangeStatus error
		errorMsg       string
	}{
		{
			"405",
			http.MethodPost,
			"/api/exchange-status",
			http.StatusMethodNotAllowed,
			"Invalid request method",
			nil,
			"",
		},

		{
			"200",
			http.MethodGet,
			"/api/exchange-status",
			http.StatusOK,
			"",
			nil,
			"",
		},

		{
			"200 status message error",
			http.MethodGet,
			"/api/exchange-status",
			http.StatusOK,
			"",
			errors.New("exchange.Status error"),
			"exchange.Status error",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			e := &fakeExchanger{}
			e.On("Status").Return(tc.exchangeStatus)

			req, err := http.NewRequest(tc.method, tc.url, nil)
			require.NoError(t, err)

			log, _ := testutil.NewLogger(t)

			rr := httptest.NewRecorder()
			httpServ := &HTTPServer{
				log:       log,
				exchanger: e,
			}
			handler := httpServ.setupMux()
			handler.ServeHTTP(rr, req)

			status := rr.Code
			require.Equal(t, tc.status, status, "wrong status code: got `%v` want `%v`", tc.name, status, tc.status)

			if status != http.StatusOK {
				require.Equal(t, tc.err, strings.TrimSpace(rr.Body.String()))
			} else {
				var msg ExchangeStatusResponse
				err := json.Unmarshal(rr.Body.Bytes(), &msg)
				require.NoError(t, err)
				require.Equal(t, ExchangeStatusResponse{
					Error: tc.errorMsg,
				}, msg)
			}
		})
	}

}
