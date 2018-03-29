package teller

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/skycoin/skycoin/src/api/cli"

	"github.com/skycoin/teller/src/exchange"
	"github.com/skycoin/teller/src/sender"
	"github.com/skycoin/teller/src/util/testutil"
)

type fakeExchanger struct {
	mock.Mock
}

func (e *fakeExchanger) BindAddress(skyAddr, depositAddr, coinType string) (*exchange.BoundAddress, error) {
	args := e.Called(skyAddr, depositAddr, coinType)

	ba := args.Get(0)
	if ba == nil {
		return nil, args.Error(1)
	}

	return ba.(*exchange.BoundAddress), args.Error(1)
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

func (e *fakeExchanger) SenderStatus() error {
	args := e.Called()
	return args.Error(0)
}

func (e *fakeExchanger) ProcessorStatus() error {
	args := e.Called()
	return args.Error(0)
}

func (e *fakeExchanger) ErroredDeposits() ([]exchange.DepositInfo, error) {
	args := e.Called()
	return args.Get(0).([]exchange.DepositInfo), args.Error(1)
}

func (e *fakeExchanger) Balance() (*cli.Balance, error) {
	args := e.Called()

	b := args.Get(0)
	if b == nil {
		return nil, args.Error(1)
	}

	return b.(*cli.Balance), args.Error(1)
}

func TestHealthHandler(t *testing.T) {
	tt := []struct {
		name               string
		method             string
		url                string
		status             int
		err                string
		senderStatus       error
		processorStatus    error
		senderErrMsg       string
		balance            cli.Balance
		balanceError       error
		erroredDeposits    []exchange.DepositInfo
		erroredDepositsErr error
	}{
		{
			name:   "405",
			method: http.MethodPost,
			url:    "/api/health",
			status: http.StatusMethodNotAllowed,
			err:    "Invalid request method",
			balance: cli.Balance{
				Coins: "100.000000",
				Hours: "100",
			},
		},

		{
			name:   "200 with errored deposits",
			method: http.MethodGet,
			url:    "/api/health",
			status: http.StatusOK,
			balance: cli.Balance{
				Coins: "100.000000",
				Hours: "100",
			},
			erroredDeposits: make([]exchange.DepositInfo, 3),
		},

		{
			name:         "200 status message error ignored, not RPCError",
			method:       http.MethodGet,
			url:          "/api/health",
			status:       http.StatusOK,
			senderStatus: errors.New("exchange.Status error"),
			balance: cli.Balance{
				Coins: "100.000000",
				Hours: "100",
			},
		},

		{
			name:         "200 status message error is RPCError",
			method:       http.MethodGet,
			url:          "/api/health",
			status:       http.StatusOK,
			senderStatus: sender.NewRPCError(errors.New("exchange.Status RPC error")),
			senderErrMsg: "exchange.Status RPC error",
			balance: cli.Balance{
				Coins: "100.000000",
				Hours: "100",
			},
		},

		{
			name:            "200 status message with processor error",
			method:          http.MethodGet,
			url:             "/api/health",
			status:          http.StatusOK,
			processorStatus: errors.New("processor error"),
			balance: cli.Balance{
				Coins: "100.000000",
				Hours: "100",
			},
		},

		{
			name:   "200 cli balance error and errored deposits error",
			method: http.MethodGet,
			url:    "/api/health",
			status: http.StatusOK,
			balance: cli.Balance{
				Coins: "0.000000",
				Hours: "0",
			},
			balanceError:       errors.New("cli balance error"),
			erroredDepositsErr: errors.New("errored deposits"),
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			e := &fakeExchanger{}

			e.On("SenderStatus").Return(tc.senderStatus)
			e.On("ProcessorStatus").Return(tc.processorStatus)
			e.On("ErroredDeposits").Return(tc.erroredDeposits, tc.erroredDepositsErr)

			if tc.balanceError == nil {
				e.On("Balance").Return(&tc.balance, nil)
			} else {
				e.On("Balance").Return(nil, tc.balanceError)
			}

			req, err := http.NewRequest(tc.method, tc.url, nil)
			require.NoError(t, err)

			log, _ := testutil.NewLogger(t)

			rr := httptest.NewRecorder()
			httpServ := &HTTPServer{
				log:       log,
				exchanger: e,
			}
			httpServ.cfg.GitCommit = "git-commit-hash"
			httpServ.cfg.StartTime = time.Now()

			handler := httpServ.setupMux()

			handler.ServeHTTP(rr, req)

			status := rr.Code
			require.Equal(t, tc.status, status, "wrong status code: got `%v` want `%v`", tc.name, status, tc.status)

			if status != http.StatusOK {
				require.Equal(t, tc.err, strings.TrimSpace(rr.Body.String()))
				return
			}

			processorErrMsg := ""
			if tc.processorStatus != nil {
				processorErrMsg = tc.processorStatus.Error()
			}

			var msg HealthResponse
			err = json.Unmarshal(rr.Body.Bytes(), &msg)
			require.NoError(t, err)
			require.Equal(t, ExchangeStatusResponse{
				DepositErrorCount: len(tc.erroredDeposits),
				ProcessorError:    processorErrMsg,
				SenderError:       tc.senderErrMsg,
				Balance: ExchangeStatusResponseBalance{
					Coins: tc.balance.Coins,
					Hours: tc.balance.Hours,
				},
			}, msg.ExchangeStatus)
			require.Equal(t, "git-commit-hash", msg.Version)

			uptime, err := time.ParseDuration(msg.Uptime)
			require.NoError(t, err)
			require.True(t, uptime > 0)
		})
	}

}
