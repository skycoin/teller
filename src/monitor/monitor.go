// Package monitor service provides http apis to query the teller
// resouces
package monitor

import (
	"context"
	"fmt"
	"net/http"
	"time"
	"encoding/json"

	"github.com/sirupsen/logrus"

	"github.com/MDLlife/teller/src/exchange"
	"github.com/MDLlife/teller/src/util/httputil"
	"github.com/MDLlife/teller/src/util/logger"
	"github.com/MDLlife/teller/src/util/mathutil"
	"github.com/shopspring/decimal"
)

const (
	shutdownTimeout = time.Second * 5

	// https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/
	// The timeout configuration is necessary for public servers, or else
	// connections will be used up
	serverReadTimeout  = time.Second * 10
	serverWriteTimeout = time.Second * 60
	serverIdleTimeout  = time.Second * 120

	cryptocompareFrequency = time.Minute * 5
)

// AddrManager interface provides apis to access resource of btc address
type AddrManager interface {
	Remaining() uint64 // returns the rest number of btc address in the pool
}

// DepositStatusGetter  interface provides api to access exchange resource
type DepositStatusGetter interface {
	GetDepositStatusDetail(flt exchange.DepositFilter) ([]exchange.DepositStatusDetail, error)
	GetDepositStats() (*exchange.DepositStats, error)
}

// ScanAddressGetter get scanning address interface
type ScanAddressGetter interface {
	GetScanAddresses() ([]string, error)
}

// WebReadyStats deposit struct for api
type WebReadyStats struct {
	TotalBTCReceived   string `json:"btc"`
	TotalETHReceived   string `json:"eth"`
	TotalSKYReceived   string `json:"sky"`
	TotalWAVESReceived string `json:"waves"`
	TotalUSDReceived   string `json:"usd"`
	TotalMDLSent       string `json:"mdl"`
	TotalTransactions  int64  `json:"tx"`
}

// ETH course
type CryptocompareResponse struct {
	Response string              `json:"Response"`
	Data     []CryptocompareData `json:"Data"`
}

type CryptocompareData struct {
	Time       uint32  `json:"time"`
	Close      float32 `json:"close"`
	High       float32 `json:"high"`
	Low        float32 `json:"low"`
	Open       float32 `json:"open"`
	VolumeFrom float32 `json:"volumefrom"`
	VolumeTo   float32 `json:"volumeto"`
}

var cryptocompareETHtoUSDcourse float32 = 510.0
var cryptocompareUpdateTime = time.Now().Add(-time.Hour)

// Config configuration info for monitor service
type Config struct {
	Addr          string
	FixBtcValue   int64
	FixEthValue   int64
	FixSkyValue   int64
	FixWavesValue int64
	FixMdlValue   int64
	FixUsdValue   decimal.Decimal
	FixTxValue    int64
}

// Monitor monitor service struct
type Monitor struct {
	log logrus.FieldLogger
	AddrManager
	EthAddrManager   AddrManager
	SkyAddrManager   AddrManager
	WavesAddrManager AddrManager
	DepositStatusGetter
	ScanAddressGetter
	cfg  Config
	ln   *http.Server
	quit chan struct{}
}

// New creates monitor service
func New(log logrus.FieldLogger, cfg Config, addrManager, ethAddrManager AddrManager, skyAddrManager AddrManager, wavesAddrManager AddrManager, dpstget DepositStatusGetter, sag ScanAddressGetter) *Monitor {
	return &Monitor{
		log:                 log.WithField("prefix", "teller.monitor"),
		cfg:                 cfg,
		AddrManager:         addrManager,
		EthAddrManager:      ethAddrManager,
		SkyAddrManager:      skyAddrManager,
		WavesAddrManager:    wavesAddrManager,
		DepositStatusGetter: dpstget,
		ScanAddressGetter:   sag,
		quit:                make(chan struct{}),
	}
}

// Run starts the monitor service
func (m *Monitor) Run() error {
	log := m.log.WithField("config", m.cfg)
	log.Info("Start monitor service...")
	defer log.Info("Monitor Service closed")

	mux := m.setupMux()

	m.ln = &http.Server{
		Addr:         m.cfg.Addr,
		Handler:      mux,
		ReadTimeout:  serverReadTimeout,
		WriteTimeout: serverWriteTimeout,
		IdleTimeout:  serverIdleTimeout,
	}

	if err := m.ln.ListenAndServe(); err != nil {
		select {
		case <-m.quit:
			return nil
		default:
			return err
		}
	}
	return nil
}

func (m *Monitor) setupMux() *http.ServeMux {
	mux := http.NewServeMux()

	mux.Handle("/api/address", httputil.LogHandler(m.log, m.addressHandler()))
	mux.Handle("/api/deposit_status", httputil.LogHandler(m.log, m.depositStatus()))
	mux.Handle("/api/stats", httputil.LogHandler(m.log, m.statsHandler()))
	mux.Handle("/api/web-stats", httputil.LogHandler(m.log, m.webStatsHandler()))
	mux.Handle("/api/eth-total-stats", httputil.LogHandler(m.log, m.ethTotalStatsHandler()))
	return mux
}

// Shutdown close the monitor service
func (m *Monitor) Shutdown() {
	log := m.log.WithField("timeout", shutdownTimeout)
	defer log.Info("Shutdown monitor service")

	close(m.quit)
	if m.ln != nil {
		log.Info("Shutting down monitor service")
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := m.ln.Shutdown(ctx); err != nil {
			log.WithError(err).Error("Monitor service shutdown failed")
		}
	}
}

type addressUsage struct {
	RestAddrNum   uint64   `json:"rest_address_num"`
	ScanningAddrs []string `json:"scanning_addresses"`
}

// addressHandler returns the btc address usage
// Method: GET
// URI: /api/address
func (m *Monitor) addressHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := logger.FromContext(ctx)

		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			httputil.ErrResponse(w, http.StatusMethodNotAllowed)
			return
		}

		addrs, err := m.GetScanAddresses()
		if err != nil {
			log.WithError(err).Error("GetScanAddresses failed")
			httputil.ErrResponse(w, http.StatusInternalServerError)
			return
		}

		addrUsage := addressUsage{
			RestAddrNum:   m.Remaining(),
			ScanningAddrs: addrs,
		}

		if err := httputil.JSONResponse(w, addrUsage); err != nil {
			log.WithError(err).Error("Write json response failed")
			return
		}
	}
}

// depositStatus returns all deposit status
// Method: GET
// URI: /api/deposit_status
// Args:
//     - status # available value("waiting_deposit", "waiting_send", "waiting_confirm", "done")
func (m *Monitor) depositStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := logger.FromContext(ctx)

		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			httputil.ErrResponse(w, http.StatusMethodNotAllowed)
			return
		}

		status := r.FormValue("status")
		if status == "" {
			// returns all status
			dpis, err := m.GetDepositStatusDetail(func(dpi exchange.DepositInfo) bool {
				return true
			})
			if err != nil {
				log.WithError(err).Error("GetDepositStatusDetail failed")
				httputil.ErrResponse(w, http.StatusInternalServerError)
				return
			}
			if err := httputil.JSONResponse(w, dpis); err != nil {
				log.WithError(err).Error("Write json response failed")
				return
			}
			return
		}

		st := exchange.NewStatusFromStr(status)
		switch st {
		case exchange.StatusUnknown:
			err := fmt.Sprintf("unknown status %v", status)
			httputil.ErrResponse(w, http.StatusBadRequest, err)
			log.WithField("depositStatus", status).Error("Unknown status")
			return
		default:
			dpis, err := m.GetDepositStatusDetail(func(dpi exchange.DepositInfo) bool {
				return dpi.Status == st
			})
			if err != nil {
				log.WithError(err).Error("GetDepositStatusDetail failed")
				httputil.ErrResponse(w, http.StatusInternalServerError)
				return
			}

			if err := httputil.JSONResponse(w, dpis); err != nil {
				log.WithError(err).Error("Write json response failed")
				return
			}
		}
	}
}

// stats returns all deposit stats, including total BTC received and total MDL sent.
// Method: GET
// URI: /api/stats
func (m *Monitor) statsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := logger.FromContext(ctx)

		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			httputil.ErrResponse(w, http.StatusMethodNotAllowed)
			return
		}

		ts, err := m.GetDepositStats()
		if err != nil {
			log.WithError(err).Error("GetDepositStats failed")
			httputil.ErrResponse(w, http.StatusInternalServerError)
			return
		}

		if err := httputil.JSONResponse(w, ts); err != nil {
			log.WithError(err).Error("Write json response failed")
			return
		}
	}
}

var updateEthToUSDCourse = func(log logrus.FieldLogger) {
	if cryptocompareUpdateTime.After(time.Now().Add(-cryptocompareFrequency)) {
		return
	}

	timeout := time.Duration(1 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}
	rsp, err := client.Get("https://min-api.cryptocompare.com/data/histominute?fsym=ETH&limit=1&tsym=USD")
	if err != nil {
		log.Error("Can't connect to https://min-api.cryptocompare.com/data/histominute?fsym=ETH&limit=1&tsym=USD Error: " + err.Error())
	} else {
		if rsp.StatusCode == http.StatusOK {
			defer rsp.Body.Close()

			var jResponse CryptocompareResponse
			if err := json.NewDecoder(rsp.Body).Decode(&jResponse); err != nil {
				log.Error("Can't parse min-api.cryptocompare json response")
			} else {
				if len(jResponse.Data) > 0 {
					cryptocompareETHtoUSDcourse = jResponse.Data[0].Close
					cryptocompareUpdateTime = time.Now()
				}
			}
		} else {
			log.Error("Response Status of  min-api.cryptocompare :" + rsp.Status + "\n\t body: ")
		}
	}
}

// stats returns all deposit stats prepared for web, including total BTC, ETH, SKY, WAVES received, total MDL sent and USD approximately received based on MDL.
// Method: GET
// URI: /api/web-stats
func (m *Monitor) webStatsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := logger.FromContext(ctx)

		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			httputil.ErrResponse(w, http.StatusMethodNotAllowed)
			return
		}

		ts, err := m.GetDepositStats()
		if err != nil {
			log.WithError(err).Error("GetDepositStats failed")
			httputil.ErrResponse(w, http.StatusInternalServerError)
			return
		}

		ts.TotalBTCReceived = ts.TotalBTCReceived + m.cfg.FixBtcValue
		ts.TotalETHReceived = ts.TotalETHReceived + m.cfg.FixEthValue
		ts.TotalSKYReceived = ts.TotalSKYReceived + m.cfg.FixSkyValue
		ts.TotalWAVESReceived = ts.TotalWAVESReceived + m.cfg.FixWavesValue
		ts.TotalMDLSent = ts.TotalMDLSent + m.cfg.FixMdlValue
		ts.TotalTransactions = ts.TotalTransactions + m.cfg.FixTxValue

		mdl := mathutil.IntToMDL(ts.TotalMDLSent)
		usd := mdl.Mul(decimal.NewFromFloat(0.05)).Add(m.cfg.FixUsdValue)

		ws := &WebReadyStats{
			mathutil.IntToBTC(ts.TotalBTCReceived).String(),
			mathutil.IntToETH(ts.TotalETHReceived).String(),
			mathutil.IntToSKY(ts.TotalSKYReceived).String(),
			mathutil.IntToWAV(ts.TotalWAVESReceived).String(),
			usd.String(),
			mdl.String(),
			ts.TotalTransactions,
		}

		if err := httputil.JSONResponse(w, ws); err != nil {
			log.WithError(err).Error("Write json response failed")
			return
		}
	}
}

// stats returns all deposit stats prepared for web, including total BTC, ETH, SKY, WAVES received, total MDL sent and USD approximately received based on MDL.
// Method: GET
// URI: /api/eth-total-stats
func (m *Monitor) ethTotalStatsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := logger.FromContext(ctx)

		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			httputil.ErrResponse(w, http.StatusMethodNotAllowed)
			return
		}

		ts, err := m.GetDepositStats()
		if err != nil {
			log.WithError(err).Error("GetDepositStats failed")
			httputil.ErrResponse(w, http.StatusInternalServerError)
			return
		}

		// update course
		updateEthToUSDCourse(log)

		mdl := mathutil.IntToMDL(ts.TotalMDLSent)
		usd := mdl.Mul(decimal.NewFromFloat(0.05)).Add(m.cfg.FixUsdValue)
		eth := usd.Div(decimal.NewFromFloat(float64(cryptocompareETHtoUSDcourse)))

		if err := httputil.JSONResponse(w, map[string]string{"eth": eth.String()}); err != nil {
			log.WithError(err).Error("Write json response failed")
			return
		}
	}
}
