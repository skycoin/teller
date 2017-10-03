// Package monitor service provides http apis to query the teller
// resouces
package monitor

import (
	"context"
	"net/http"
	"time"

	"fmt"

	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/util/httputil"
	"github.com/skycoin/teller/src/util/logger"
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
	logger.Logger
	BtcAddrManager
	DepositStatusGetter
	ScanAddressGetter
	cfg  Config
	ln   *http.Server
	quit chan struct{}
}

// New creates monitor service
func New(cfg Config,
	log logger.Logger,
	btcAddrMgr BtcAddrManager,
	dpstget DepositStatusGetter,
	sag ScanAddressGetter) *Monitor {
	return &Monitor{
		Logger:              log,
		cfg:                 cfg,
		BtcAddrManager:      btcAddrMgr,
		DepositStatusGetter: dpstget,
		ScanAddressGetter:   sag,
		quit:                make(chan struct{}),
	}
}

// Run starts the monitor service
func (m *Monitor) Run() error {
	m.Println("Start monitor service...")
	defer m.Println("Monitor Service closed")

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

	mux.HandleFunc("/api/address", m.addressHandler())
	mux.HandleFunc("/api/deposit_status", m.depositStatus())
	return mux
}

// Shutdown close the monitor service
func (m *Monitor) Shutdown() {
	close(m.quit)
	if m.ln != nil {
		m.Printf("Shutting down monitor service, %s timeout\n", shutdownTimeout)
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := m.ln.Shutdown(ctx); err != nil {
			m.Println("Monitor service shutdown error:", err)
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
		if r.Method != "GET" {
			w.Header().Set("Allow", "GET")
			httputil.ErrResponse(w, http.StatusMethodNotAllowed)
			m.Println(http.StatusText(http.StatusMethodNotAllowed))
			return
		}

		addrs, err := m.GetScanAddresses()
		if err != nil {
			m.Println("GetScanAddresses failed:", err)
			httputil.ErrResponse(w, http.StatusInternalServerError)
			return
		}

		addrUsage := addressUsage{
			RestAddrNum:   m.RestNum(),
			ScanningAddrs: addrs,
		}

		if err := httputil.JSONResponse(w, addrUsage); err != nil {
			m.Println("Write json response failed:", err)
			return
		}
	}
}

func getAll(dpi exchange.DepositInfo) bool {
	return true
}

// depostStatus returns all deposit status
// Method: GET
// URI: /api/deposit_status
// Args:
//     - status # available value("waiting_deposit", "waiting_send", "waiting_confirm", "done")
func (m *Monitor) depositStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.Header().Set("Allow", "GET")
			httputil.ErrResponse(w, http.StatusMethodNotAllowed)
			m.Println(http.StatusText(http.StatusMethodNotAllowed))
			return
		}
		status := r.FormValue("status")
		if status == "" {
			// returns all status
			dpis, err := m.GetDepositStatusDetail(getAll)
			if err != nil {
				httputil.ErrResponse(w, http.StatusInternalServerError)
				m.Println(err)
				return
			}
			httputil.JSONResponse(w, dpis)
			return
		}

		st := exchange.NewStatusFromStr(status)
		switch st {
		case exchange.StatusUnknow:
			httputil.ErrResponse(w, http.StatusBadRequest, fmt.Sprintf("unknow status %v", status))
			m.Println("Unknow status", status)
			return
		default:
			dpis, err := m.GetDepositStatusDetail(func(dpi exchange.DepositInfo) bool {
				return dpi.Status == st
			})
			if err != nil {
				httputil.ErrResponse(w, http.StatusInternalServerError)
				m.Println(err)
				return
			}

			httputil.JSONResponse(w, dpis)
		}
	}
}
