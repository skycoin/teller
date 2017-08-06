package proxy

import (
	"context"
	"fmt"
	"net/http"

	"time"

	"github.com/NYTimes/gziphandler"

	"github.com/skycoin/skycoin/src/cipher"
	"github.com/skycoin/teller/src/daemon"
	"github.com/skycoin/teller/src/httputil"
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
)

// var httpErrCodeStr = []string{
// 	http.StatusBadRequest:          "Bad Request",
// 	http.StatusMethodNotAllowed:    "Method Not Allowed",
// 	http.StatusNotAcceptable:       "Not Acceptable",
// 	http.StatusInternalServerError: "Internal Server Error",
// 	http.StatusRequestTimeout:      "Request Timeout",
// }

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
		mux.Handle(path, gziphandler.GzipHandler(httputil.LogHandler(hs.Logger, f)))
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
			hs.Println("HTTP server shutdown error:", err)
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
			httputil.ErrResponse(w, http.StatusMethodNotAllowed)
			gw.Println(http.StatusText(http.StatusMethodNotAllowed))
			return
		}

		skyAddr := r.FormValue("skyaddr")
		if skyAddr == "" {
			httputil.ErrResponse(w, http.StatusBadRequest)
			gw.Println(http.StatusText(http.StatusBadRequest))
			return
		}

		// verify the skycoin address
		if _, err := cipher.DecodeBase58Address(skyAddr); err != nil {
			http.Error(w, fmt.Sprintf("Invalid skycoin address: %v", err), http.StatusBadRequest)
			gw.Println("Invalid skycoin address:", err)
			return
		}

		cxt, cancel := context.WithTimeout(r.Context(), proxyRequestTimeout)
		defer cancel()

		bindReq := daemon.BindRequest{SkyAddress: skyAddr}

		rsp, err := gw.BindAddress(cxt, &bindReq)
		if err != nil {
			if err == context.DeadlineExceeded {
				httputil.ErrResponse(w, http.StatusRequestTimeout)
				gw.Println(http.StatusText(http.StatusRequestTimeout))
				return
			}

			httputil.ErrResponse(w, http.StatusInternalServerError)
			gw.Println(err)
			return
		}

		if err := httputil.JSONResponse(w, makeBindHTTPResponse(*rsp)); err != nil {
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
			httputil.ErrResponse(w, http.StatusMethodNotAllowed)
			gw.Println(http.StatusText(http.StatusMethodNotAllowed))
			return
		}

		skyAddr := r.FormValue("skyaddr")
		if skyAddr == "" {
			httputil.ErrResponse(w, http.StatusBadRequest)
			gw.Println(http.StatusText(http.StatusBadRequest))
			return
		}

		// verify the skycoin address
		if _, err := cipher.DecodeBase58Address(skyAddr); err != nil {
			http.Error(w, fmt.Sprintf("Invalid skycoin address: %v", err), http.StatusBadRequest)
			gw.Println("Invalid skycoin address:", err)
			return
		}

		cxt, cancel := context.WithTimeout(r.Context(), proxyRequestTimeout)
		defer cancel()

		stReq := daemon.StatusRequest{SkyAddress: skyAddr}

		rsp, err := gw.GetDepositStatuses(cxt, &stReq)
		if err != nil {
			if err == context.DeadlineExceeded {
				httputil.ErrResponse(w, http.StatusRequestTimeout)
				gw.Println(http.StatusText(http.StatusRequestTimeout))
				return
			}

			httputil.ErrResponse(w, http.StatusInternalServerError)
			gw.Println(err)
			return
		}

		if err := httputil.JSONResponse(w, makeStatusHTTPResponse(*rsp)); err != nil {
			gw.Println(err)
		}
	}
}
