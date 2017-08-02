package proxy

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"encoding/json"

	"time"

	"github.com/NYTimes/gziphandler"

	"github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/logger"
)

const (
	errBadRequest       = 400
	errMethodNotAllowed = 405
	errNotAcceptable    = 406
	errRequestTimeout   = 408
	errInternalServErr  = 500

	maxRequestLogsNum = 100

	proxyRequestTimeout = 3 * time.Second

	shutdownTimeout = time.Second * 5

	// https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/
	// The timeout configuration is necessary for public servers, or else
	// connections will be used up
	serverReadTimeout  = time.Second * 10
	serverWriteTimeout = time.Second * 60
	serverIdleTimeout  = time.Second * 120
)

var httpErrCodeStr = []string{
	errBadRequest:       "Bad Request",
	errMethodNotAllowed: "Method Not Allowed",
	errNotAcceptable:    "Not Acceptable",
	errInternalServErr:  "Internal Server Error",
	errRequestTimeout:   "Request Timeout",
}

type httpServ struct {
	logger.Logger
	Addr          string
	StaticDir     string
	HtmlInterface bool
	ln            *http.Server
	gateway       *gateway
	quit          chan struct{}
}

// creates a http service
func newHTTPServ(addr string, log logger.Logger, gw *gateway) *httpServ {
	return &httpServ{
		Logger:        log,
		Addr:          addr,
		StaticDir:     "./web/build/",
		HtmlInterface: true,
		gateway:       gw,
		quit:          make(chan struct{}),
	}
}

func (hs *httpServ) Run() error {
	hs.Println("Http service start, serve on", hs.Addr)
	defer hs.Debugln("Http service closed")

	mux := hs.setupMux()

	hs.ln = &http.Server{
		Addr:         hs.Addr,
		Handler:      mux,
		ReadTimeout:  serverReadTimeout,
		WriteTimeout: serverWriteTimeout,
		IdleTimeout:  serverIdleTimeout,
	}

	if err := hs.ln.ListenAndServe(); err != nil {
		select {
		case <-hs.quit:
			return nil
		default:
			return fmt.Errorf("http serve failed: %v", err)
		}
	}

	return nil
}

func (hs *httpServ) setupMux() *http.ServeMux {
	mux := http.NewServeMux()

	handleApi := func(path string, f http.HandlerFunc) {
		mux.Handle(path, gziphandler.GzipHandler(logHandler(hs.Logger, f)))
	}

	// API Methods
	handleApi("/api/bind", BindHandler(hs.gateway))
	handleApi("/api/status", StatusHandler(hs.gateway))

	// Static files
	if hs.HtmlInterface {
		mux.Handle("/", gziphandler.GzipHandler(http.FileServer(http.Dir(hs.StaticDir))))
	}

	return mux
}

func (hs *httpServ) Shutdown() {
	close(hs.quit)

	if hs.ln != nil {
		hs.Printf("Shutting down http server, %s timeout\n", shutdownTimeout)
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := hs.ln.Shutdown(ctx); err != nil {
			log.Println("HTTP server shutdown error:", err)
		}
	}
}

// BindHandler binds skycoin address with a bitcoin address
// Method: GET
// URI: /api/bind
// Args:
//    skyaddr
func BindHandler(gw gatewayer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.Header().Set("Allow", "GET")
			errorResponse(w, http.StatusMethodNotAllowed)
			gw.Println(httpErrCodeStr[http.StatusMethodNotAllowed])
			return
		}

		skyAddr := r.FormValue("skyaddr")
		if skyAddr == "" {
			errorResponse(w, http.StatusBadRequest)
			gw.Println(httpErrCodeStr[http.StatusBadRequest])
			return
		}

		// verify the skycoin address
		if _, err := cipher.DecodeBase58Address(skyAddr); err != nil {
			http.Error(w, fmt.Sprintf("Invalid skycoin address: %v", err), http.StatusBadRequest)
			gw.Println("Invalid skycoin address:", err)
			return
		}

		bindReq := daemon.BindRequest{SkyAddress: skyAddr}

		cxt, cancel := context.WithTimeout(r.Context(), proxyRequestTimeout)
		defer cancel()
		rsp, err := gw.BindAddress(cxt, &bindReq)
		if err != nil {
			if err == context.DeadlineExceeded {
				errorResponse(w, http.StatusRequestTimeout)
				gw.Println(httpErrCodeStr[http.StatusRequestTimeout])
				return
			}
			errorResponse(w, http.StatusInternalServerError)
			gw.Println(err)
			return
		}

		if err := jsonResponse(w, makeBindHTTPResponse(*rsp)); err != nil {
			gw.Println(err)
		}
	}
}

// StatusHandler returns the deposit status of specific skycoin address
// Method: GET
// URI: /api/status
// Args:
//     skyaddr
func StatusHandler(gw gatewayer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.Header().Set("Allow", "GET")
			errorResponse(w, http.StatusMethodNotAllowed)
			gw.Println(httpErrCodeStr[http.StatusMethodNotAllowed])
			return
		}

		skyAddr := r.FormValue("skyaddr")
		if skyAddr == "" {
			errorResponse(w, http.StatusBadRequest)
			gw.Println(httpErrCodeStr[http.StatusBadRequest])
			return
		}

		// verify the skycoin address
		if _, err := cipher.DecodeBase58Address(skyAddr); err != nil {
			http.Error(w, fmt.Sprintf("Invalid skycoin address: %v", err), http.StatusBadRequest)
			gw.Println("Invalid skycoin address:", err)
			return
		}

		stReq := daemon.StatusRequest{SkyAddress: skyAddr}
		cxt, cancel := context.WithTimeout(r.Context(), proxyRequestTimeout)
		defer cancel()
		rsp, err := gw.GetDepositStatuses(cxt, &stReq)
		if err != nil {
			if err == context.DeadlineExceeded {
				errorResponse(w, http.StatusRequestTimeout)
				gw.Println(httpErrCodeStr[http.StatusRequestTimeout])
				return
			}
			errorResponse(w, http.StatusInternalServerError)
			gw.Println(err)
			return
		}

		if err := jsonResponse(w, makeStatusHTTPResponse(*rsp)); err != nil {
			gw.Println(err)
		}
	}
}

func errorResponse(w http.ResponseWriter, code int) {
	http.Error(w, httpErrCodeStr[code], code)
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
