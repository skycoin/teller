// Package monitor service provides http apis to query the teller
// resouces
package monitor

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/MDLlife/teller/src/exchange"
	"github.com/MDLlife/teller/src/util/httputil"
	"github.com/MDLlife/teller/src/util/logger"
	"github.com/shopspring/decimal"
	"github.com/MDLlife/teller/src/util/mathutil"
)

const (
	shutdownTimeout = time.Second * 5

	// https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/
	// The timeout configuration is necessary for public servers, or else
	// connections will be used up
	serverReadTimeout  = time.Second * 10
	serverWriteTimeout = time.Second * 60
	serverIdleTimeout  = time.Second * 120
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
	TotalBTCReceived  string `json:"btc"`
	TotalETHReceived  string `json:"eth"`
	TotalSKYReceived  string `json:"sky"`
	TotalWAVEReceived string `json:"waves"`
	TotalUSDReceived  string `json:"usd"`
	TotalMDLSent      string `json:"mdl"`
}

// Config configuration info for monitor service
type Config struct {
	Addr          string
	FixBtcValue   int64
	FixEthValue   int64
	FixSkyValue   int64
	FixWavesValue int64
	FixMdlValue   int64
	FixUsdValue   string
}

// Monitor monitor service struct
type Monitor struct {
	log            logrus.FieldLogger
	AddrManager
	EthAddrManager AddrManager
	SkyAddrManager AddrManager
	DepositStatusGetter
	ScanAddressGetter
	cfg            Config
	ln             *http.Server
	quit           chan struct{}
}

// New creates monitor service
func New(log logrus.FieldLogger, cfg Config, addrManager, ethAddrManager AddrManager, skyAddrManager AddrManager, dpstget DepositStatusGetter, sag ScanAddressGetter) *Monitor {
	return &Monitor{
		log:                 log.WithField("prefix", "teller.monitor"),
		cfg:                 cfg,
		AddrManager:         addrManager,
		EthAddrManager:      ethAddrManager,
		SkyAddrManager:      skyAddrManager,
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
		ts.TotalWAVEReceived = ts.TotalWAVEReceived + m.cfg.FixWavesValue
		ts.TotalMDLSent = ts.TotalMDLSent + m.cfg.FixMdlValue

		mdl := mathutil.IntToMDL(ts.TotalMDLSent)
		fixUsd, err := mathutil.DecimalFromString(m.cfg.FixUsdValue)
		if err != nil {
			fixUsd = decimal.New(0, 0)
			m.log.Error("Can't convert fix_usd_value: " + m.cfg.FixUsdValue + " to decimal")
		}
		usd := mdl.Mul(decimal.NewFromFloat(0.05)).Add(fixUsd)

		ws := &WebReadyStats{
			mathutil.IntToBTC(ts.TotalBTCReceived).String(),
			mathutil.IntToETH(ts.TotalETHReceived).String(),
			mathutil.IntToSKY(ts.TotalSKYReceived).String(),
			mathutil.IntToWAV(ts.TotalWAVEReceived).String(),
			usd.String(),
			mdl.String(),
		}

		if err := httputil.JSONResponse(w, ws); err != nil {
			log.WithError(err).Error("Write json response failed")
			return
		}
	}
}
