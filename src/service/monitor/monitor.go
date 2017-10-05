// Package monitor service provides http apis to query the teller
// resouces
package monitor

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/httputil"
	"github.com/skycoin/teller/src/logger"
	"github.com/skycoin/teller/src/service/exchange"
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

// BtcAddrManager interface provides apis to access resource of btc address
type BtcAddrManager interface {
	RestNum() uint64 // returns the rest number of btc address in the pool
}

// DepositStatusGetter  interface provides api to access exchange resource
type DepositStatusGetter interface {
	GetDepositStatusDetail(flt exchange.DepositFilter) ([]daemon.DepositStatusDetail, error)
}

// ScanAddressGetter get scanning address interface
type ScanAddressGetter interface {
	GetScanAddresses() ([]string, error)
}

// Config configuration info for monitor service
type Config struct {
	Addr string
}

// Monitor monitor service struct
type Monitor struct {
	log logrus.FieldLogger
	BtcAddrManager
	DepositStatusGetter
	ScanAddressGetter
	cfg  Config
	ln   *http.Server
	quit chan struct{}
}

// New creates monitor service
func New(cfg Config, log logrus.FieldLogger, btcAddrMgr BtcAddrManager, dpstget DepositStatusGetter, sag ScanAddressGetter) *Monitor {
	return &Monitor{
		log: log.WithFields(logrus.Fields{
			"prefix": "monitor",
			"obj":    "Monitor",
		}),
		cfg:                 cfg,
		BtcAddrManager:      btcAddrMgr,
		DepositStatusGetter: dpstget,
		ScanAddressGetter:   sag,
		quit:                make(chan struct{}),
	}
}

// Run starts the monitor service
func (m *Monitor) Run() error {
	log := m.log.WithField("func", "Run")

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

	mux.HandleFunc("/api/address", httputil.LogHandler(m.log, m.addressHandler()))
	mux.HandleFunc("/api/deposit_status", httputil.LogHandler(m.log, m.depositStatus()))
	return mux
}

// Shutdown close the monitor service
func (m *Monitor) Shutdown() {
	log := m.log.WithFields(logrus.Fields{
		"timeout": shutdownTimeout,
		"func":    "Shutdown",
	})

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
			RestAddrNum:   m.RestNum(),
			ScanningAddrs: addrs,
		}

		if err := httputil.JSONResponse(w, addrUsage); err != nil {
			log.WithError(err).Error("Write json response failed")
			return
		}
	}
}

// depostStatus returns all deposit status
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
			httputil.JSONResponse(w, dpis)
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

			httputil.JSONResponse(w, dpis)
		}
	}
}
