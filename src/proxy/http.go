package proxy

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"strings"

	"encoding/json"

	"time"

	"github.com/NYTimes/gziphandler"
	"golang.org/x/crypto/acme/autocert"

	"github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/logger"
)

const (
	proxyRequestTimeout = time.Second * 5

	shutdownTimeout = time.Second * 5

	// https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/
	// The timeout configuration is necessary for public servers, or else
	// connections will be used up
	serverReadTimeout  = time.Second * 10
	serverWriteTimeout = time.Second * 60
	serverIdleTimeout  = time.Second * 120

	// Directory where cached SSL certs from Let's Encrypt are stored
	tlsAutoCertCache = "cert-cache"
)

var httpCodeNames = []string{
	http.StatusBadRequest:           "Bad Request",
	http.StatusMethodNotAllowed:     "Method Not Allowed",
	http.StatusNotAcceptable:        "Not Acceptable",
	http.StatusInternalServerError:  "Internal Server Error",
	http.StatusRequestTimeout:       "Request Timeout",
	http.StatusUnsupportedMediaType: "Unsupported Media Type",
}

type httpServ struct {
	logger.Logger
	Addr          string
	StaticDir     string
	HtmlInterface bool
	Tls           bool
	AutoTlsHost   string
	TlsCert       string
	TlsKey        string
	Gateway       *gateway

	ln   *http.Server
	quit chan struct{}
}

func (hs *httpServ) Run() error {
	hs.Println("Http service start, serve on", hs.Addr)
	defer hs.Debugln("Http service closed")

	hs.quit = make(chan struct{})

	mux := hs.setupMux()

	hs.ln = &http.Server{
		Addr:         hs.Addr,
		Handler:      mux,
		ReadTimeout:  serverReadTimeout,
		WriteTimeout: serverWriteTimeout,
		IdleTimeout:  serverIdleTimeout,
	}

	handleListenErr := func(f func() error) error {
		if err := f(); err != nil {
			select {
			case <-hs.quit:
				return nil
			default:
				return fmt.Errorf("http serve failed: %v", err)
			}
		}
		return nil
	}

	if hs.Tls {
		if hs.AutoTlsHost != "" {
			// https://godoc.org/golang.org/x/crypto/acme/autocert
			// https://stackoverflow.com/a/40494806
			certManager := autocert.Manager{
				Prompt:     autocert.AcceptTOS,
				HostPolicy: autocert.HostWhitelist(hs.AutoTlsHost),
				Cache:      autocert.DirCache(tlsAutoCertCache),
			}

			hs.ln.TLSConfig = &tls.Config{
				GetCertificate: certManager.GetCertificate,
			}

			return handleListenErr(func() error {
				return hs.ln.ListenAndServeTLS("", "")
			})
		} else {
			return handleListenErr(func() error {
				return hs.ln.ListenAndServeTLS(hs.TlsCert, hs.TlsKey)
			})
		}

	} else {
		return handleListenErr(func() error {
			return hs.ln.ListenAndServe()
		})
	}

	return nil
}

func (hs *httpServ) setupMux() *http.ServeMux {
	mux := http.NewServeMux()

	handleApi := func(path string, f http.HandlerFunc) {
		mux.Handle(path, gziphandler.GzipHandler(logHandler(hs.Logger, f)))
	}

	// API Methods
	handleApi("/api/bind", BindHandler(hs.Gateway))
	handleApi("/api/status", StatusHandler(hs.Gateway))

	// Static files
	if hs.HtmlInterface {
		mux.Handle("/", gziphandler.GzipHandler(http.FileServer(http.Dir(hs.StaticDir))))
	}

	return mux
}

func (hs *httpServ) Shutdown() {
	if hs.quit != nil {
		close(hs.quit)
	}

	if hs.ln != nil {
		hs.Printf("Shutting down http server, %s timeout\n", shutdownTimeout)
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := hs.ln.Shutdown(ctx); err != nil {
			log.Println("HTTP server shutdown error:", err)
		}
	}

	hs.quit = nil
}

// BindHandler binds skycoin address with a bitcoin address
// Method: POST
// Accept: application/json
// URI: /api/bind
// Args:
//    {"skyaddr": "..."}
func BindHandler(gw gatewayer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bindHandler(w, r, gw)
	}
}

type bindRequest struct {
	SkyAddr string `json:"skyaddr"`
}

func bindHandler(w http.ResponseWriter, r *http.Request, gw gatewayer) {
	w.Header().Set("Accept", "application/json")

	if !validMethod(w, r, gw, []string{http.MethodPost}) {
		return
	}

	if r.Header.Get("Content-Type") != "application/json" {
		errorResponse(w, gw, http.StatusUnsupportedMediaType)
		return
	}

	userBindReq := &bindRequest{}
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&userBindReq); err != nil {
		errorResponse(w, gw, http.StatusBadRequest, "Invalid json request body:", err)
		return
	}
	defer r.Body.Close()

	if userBindReq.SkyAddr == "" {
		errorResponse(w, gw, http.StatusBadRequest, "Missing skyaddr")
		return
	}

	if !verifySkycoinAddress(w, gw, userBindReq.SkyAddr) {
		return
	}

	cxt, cancel := context.WithTimeout(r.Context(), proxyRequestTimeout)
	defer cancel()

	daemonBindReq := daemon.BindRequest{SkyAddress: userBindReq.SkyAddr}

	rsp, err := gw.BindAddress(cxt, &daemonBindReq)
	if err != nil {
		handleGatewayResponseError(w, gw, err)
		return
	}

	if err := jsonResponse(w, makeBindHTTPResponse(*rsp)); err != nil {
		gw.Println(err)
	}
}

// StatusHandler returns the deposit status of specific skycoin address
// Method: GET
// URI: /api/status
// Args:
//     skyaddr
func StatusHandler(gw gatewayer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		statusHandler(w, r, gw)
	}
}

func statusHandler(w http.ResponseWriter, r *http.Request, gw gatewayer) {
	if !validMethod(w, r, gw, []string{http.MethodGet}) {
		return
	}

	skyAddr := r.URL.Query().Get("skyaddr")
	if skyAddr == "" {
		errorResponse(w, gw, http.StatusBadRequest, "Missing skyaddr")
		return
	}

	if !verifySkycoinAddress(w, gw, skyAddr) {
		return
	}

	cxt, cancel := context.WithTimeout(r.Context(), proxyRequestTimeout)
	defer cancel()

	stReq := daemon.StatusRequest{SkyAddress: skyAddr}

	rsp, err := gw.GetDepositStatuses(cxt, &stReq)
	if err != nil {
		handleGatewayResponseError(w, gw, err)
		return
	}

	if err := jsonResponse(w, makeStatusHTTPResponse(*rsp)); err != nil {
		gw.Println(err)
	}
}

func validMethod(w http.ResponseWriter, r *http.Request, gw gatewayer, allowed []string) bool {
	for _, m := range allowed {
		if r.Method == m {
			return true
		}
	}

	w.Header().Set("Allow", strings.Join(allowed, ", "))

	status := http.StatusMethodNotAllowed
	errorResponse(w, gw, status, "Invalid request method:", r.Method)

	return false
}

func verifySkycoinAddress(w http.ResponseWriter, gw gatewayer, skyAddr string) bool {
	if _, err := cipher.DecodeBase58Address(skyAddr); err != nil {
		msg := fmt.Sprintf("Invalid skycoin address: %v", err)
		http.Error(w, msg, http.StatusBadRequest)
		gw.Println("Invalid skycoin address:", err, skyAddr)
		return false
	}
	return true
}

func handleGatewayResponseError(w http.ResponseWriter, gw gatewayer, err error) {
	if err == nil {
		return
	}

	if err == context.DeadlineExceeded {
		errorResponse(w, gw, http.StatusRequestTimeout)
		return
	}

	errorResponse(w, gw, http.StatusInternalServerError, err)
	return
}

func errorResponse(w http.ResponseWriter, gw gatewayer, code int, msgs ...interface{}) {
	http.Error(w, httpCodeNames[code], code)
	gw.Println(append([]interface{}{code, httpCodeNames[code]}, msgs...)...)
}

func jsonResponse(w http.ResponseWriter, data interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	d, err := json.MarshalIndent(data, "", "    ")
	if err != nil {
		return err
	}

	if _, err := w.Write(d); err != nil {
		return err
	}
	return nil
}

func logHandler(log logger.Logger, hd http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := time.Now()
		hd(w, r)
		log.Printf("HTTP [%s] %dms %s \n", r.Method, time.Since(t)/time.Millisecond, r.URL.String())
	}
}
