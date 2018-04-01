// Package monitor service provides http apis to query the teller
// resouces
package monitor

import (
	"context"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/skycoin/teller/src/addrs"
	"github.com/skycoin/teller/src/config"
	"github.com/skycoin/teller/src/exchange"
	"github.com/skycoin/teller/src/util/dbutil"
	"github.com/skycoin/teller/src/util/httputil"
	"github.com/skycoin/teller/src/util/logger"
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

// AddrManager interface provides an API to access deposit address statistics
type AddrManager interface {
	Remaining(string) (uint64, error)
}

// DepositStatusGetter interface provides an API to access exchange resource
type DepositStatusGetter interface {
	GetDeposits(flt exchange.DepositFilter) ([]exchange.DepositInfo, error)
	GetDepositStats() (*exchange.DepositStats, error)
	ErroredDeposits() ([]exchange.DepositInfo, error)
}

// Backuper interface provides an API to access the backup func of teller
type Backuper interface {
	Backup() http.HandlerFunc
}

// ScanAddressGetter get scanning address interface
type ScanAddressGetter interface {
	GetScanAddresses(string) ([]string, error)
}

// Monitor monitor service struct
type Monitor struct {
	log                 logrus.FieldLogger
	addrManager         AddrManager
	scanAddressGetter   ScanAddressGetter
	depositStatusGetter DepositStatusGetter
	cfg                 config.Config
	ln                  *http.Server
	Backuper
	quit chan struct{}
}

// New creates monitor service
func New(log logrus.FieldLogger, cfg config.Config, addrManager AddrManager, dpstget DepositStatusGetter, sag ScanAddressGetter, bkper Backuper) *Monitor {
	return &Monitor{
		log:                 log.WithField("prefix", "teller.monitor"),
		cfg:                 cfg,
		addrManager:         addrManager,
		depositStatusGetter: dpstget,
		scanAddressGetter:   sag,
		Backuper:            bkper,
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
		Addr:         m.cfg.AdminPanel.Host,
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

	mux.Handle("/api/deposit-addresses", httputil.LogHandler(m.log, m.depositAddressesHandler()))
	mux.Handle("/api/deposits", httputil.LogHandler(m.log, m.depositsByStatusHandler()))
	mux.Handle("/api/deposits/errored", httputil.LogHandler(m.log, m.erroredDepositsHandler()))
	mux.Handle("/api/accounting", httputil.LogHandler(m.log, m.accountingHandler()))
	mux.Handle("/api/backup", httputil.LogHandler(m.log, m.backupHandler()))
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

type depositAddressStats struct {
	ScanningEnabled       bool     `json:"scanning_enabled"`
	AddressManagerEnabled bool     `json:"address_manager_enabled"`
	RemainingAddresses    uint64   `json:"remaining_addresses"`
	ScanningAddresses     []string `json:"scanning_addresses"`
}

// depositAddressesHandler returns the btc address usage
// Method: GET
// URI: /api/address
func (m *Monitor) depositAddressesHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := logger.FromContext(ctx)

		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			httputil.ErrResponse(w, http.StatusMethodNotAllowed)
			return
		}

		resp := make(map[string]depositAddressStats, len(config.CoinTypes))

		for _, k := range config.CoinTypes {
			scanningEnabled, err := m.cfg.IsScannerEnabled(k)
			if err != nil {
				log.WithField("coinType", k).WithError(err).Error("IsScannerEnabled failed")
				httputil.ErrResponse(w, http.StatusInternalServerError)
				return
			}

			a, err := m.scanAddressGetter.GetScanAddresses(config.CoinTypeBTC)
			if err != nil {
				// If the bucket doesn't exist, this is only an error if the scanner is enabled
				// If the scanner was never enabled, the bucket won't exist.
				ignoreErr := false
				switch err.(type) {
				case dbutil.BucketNotExistErr:
					if !scanningEnabled {
						ignoreErr = true
					}
				}

				if !ignoreErr {
					log.WithField("coinType", k).WithError(err).Error("GetScanAddresses failed")
					httputil.ErrResponse(w, http.StatusInternalServerError)
					return
				}
			}

			if a == nil {
				a = []string{}
			}

			addrManagerEnabled := false
			remaining, err := m.addrManager.Remaining(k)
			switch err {
			case nil:
				addrManagerEnabled = true
			case addrs.ErrCoinTypeNotRegistered:
			default:
				log.WithField("coinType", k).WithError(err).Error("Remaining failed")
				httputil.ErrResponse(w, http.StatusInternalServerError)
				return
			}

			resp[k] = depositAddressStats{
				RemainingAddresses:    remaining,
				ScanningAddresses:     a,
				ScanningEnabled:       scanningEnabled,
				AddressManagerEnabled: addrManagerEnabled,
			}
		}

		if err := httputil.JSONResponse(w, resp); err != nil {
			log.WithError(err).Error("Write JSON response failed")
			return
		}
	}
}

type depositsByStatusResponse struct {
	Deposits []exchange.DepositInfo `json:"deposits"`
}

// depositsByStatusHandler returns all deposit status
// Method: GET
// URI: /api/deposit_status
// Args:
//    status - Optional, one of "waiting_deposit", "waiting_send", "waiting_confirm", "done", "waiting_decide", "waiting_passthrough", "waiting_passthrough_order_complete"
func (m *Monitor) depositsByStatusHandler() http.HandlerFunc {
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
			// Return all deposits
			dpis, err := m.depositStatusGetter.GetDeposits(func(dpi exchange.DepositInfo) bool {
				return true
			})
			if err != nil {
				log.WithError(err).Error("depositStatusGetter.GetDeposits failed")
				httputil.ErrResponse(w, http.StatusInternalServerError)
				return
			}

			if dpis == nil {
				dpis = []exchange.DepositInfo{}
			}

			if err := httputil.JSONResponse(w, depositsByStatusResponse{
				Deposits: dpis,
			}); err != nil {
				log.WithError(err).Error("Write JSON response failed")
				return
			}
		} else {
			if err := exchange.ValidateStatus(status); err != nil {
				httputil.ErrResponse(w, http.StatusBadRequest, "Invalid status")
				return
			}

			dpis, err := m.depositStatusGetter.GetDeposits(func(dpi exchange.DepositInfo) bool {
				return dpi.Status == status
			})
			if err != nil {
				log.WithError(err).Error("depositStatusGetter.GetDeposits failed")
				httputil.ErrResponse(w, http.StatusInternalServerError)
				return
			}

			if dpis == nil {
				dpis = []exchange.DepositInfo{}
			}

			if err := httputil.JSONResponse(w, depositsByStatusResponse{
				Deposits: dpis,
			}); err != nil {
				log.WithError(err).Error("Write JSON response failed")
				return
			}
		}
	}
}

type erroredDepositsResponse struct {
	Deposits []exchange.DepositInfo `json:"deposits"`
}

// erroredDepositsHandler returns all errored deposits
// Method: GET
// URI: /api/deposits/errored
func (m *Monitor) erroredDepositsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := logger.FromContext(ctx)

		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			httputil.ErrResponse(w, http.StatusMethodNotAllowed)
			return
		}

		deposits, err := m.depositStatusGetter.ErroredDeposits()
		if err != nil {
			log.WithError(err).Error("depositStatusGetter.ErroredDeposits failed")
			httputil.ErrResponse(w, http.StatusInternalServerError)
			return
		}

		if deposits == nil {
			deposits = []exchange.DepositInfo{}
		}

		if err := httputil.JSONResponse(w, erroredDepositsResponse{
			Deposits: deposits,
		}); err != nil {
			log.WithError(err).Error("Write JSON response failed")
			return
		}
	}
}

type accountingResponse struct {
	Sent     string            `json:"sent"`
	Received map[string]string `json:"received"`
}

// accountingHandler returns all deposit stats, including total BTC received and total SKY sent.
// Method: GET
// URI: /api/accounting
func (m *Monitor) accountingHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := logger.FromContext(ctx)

		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			httputil.ErrResponse(w, http.StatusMethodNotAllowed)
			return
		}

		stats, err := m.depositStatusGetter.GetDepositStats()
		if err != nil {
			log.WithError(err).Error("depositStatusGetter.GetDepositStats failed")
			httputil.ErrResponse(w, http.StatusInternalServerError)
			return
		}

		skySent, err := exchange.SkyAmountToString(stats.Sent)
		if err != nil {
			log.WithError(err).Error("exchange.SkyAmountToString failed")
			httputil.ErrResponse(w, http.StatusInternalServerError)
			return
		}

		received := make(map[string]string, len(stats.Received))
		for k, v := range stats.Received {
			r, err := exchange.DepositAmountToString(k, v)
			if err != nil {
				log.WithError(err).Error("exchange.DepositAmountToString failed")
				httputil.ErrResponse(w, http.StatusInternalServerError)
				return
			}
			received[k] = r
		}

		if err := httputil.JSONResponse(w, accountingResponse{
			Received: received,
			Sent:     skySent,
		}); err != nil {
			log.WithError(err).Error("Write JSON response failed")
			return
		}
	}
}

// starts downloading a database backup
// Method: GET
// URI: /api/backup
func (m *Monitor) backupHandler() http.HandlerFunc {
	return m.Backup()
}
