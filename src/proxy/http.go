package proxy

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"encoding/json"

	"time"

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
	ln      net.Listener
	srvAddr string
	gateway *gateway
	quit    chan struct{}
}

// creates a http service
func newHTTPServ(addr string, log logger.Logger, gw *gateway) *httpServ {
	return &httpServ{srvAddr: addr, Logger: log, gateway: gw, quit: make(chan struct{})}
}

func (hs *httpServ) Run() error {
	hs.Println("Http service start, serve on", hs.srvAddr)
	defer hs.Debugln("Http service closed")

	mux := http.NewServeMux()
	mux.HandleFunc("/bind", logHandler(hs.Logger, BindHandler(hs.gateway)))
	mux.HandleFunc("/status", logHandler(hs.Logger, StatusHandler(hs.gateway)))

	var err error
	hs.ln, err = net.Listen("tcp", hs.srvAddr)
	if err != nil {
		return fmt.Errorf("listen %s failed: err:%v", hs.srvAddr, err)
	}

	if err := http.Serve(hs.ln, mux); err != nil {
		select {
		case <-hs.quit:
			return nil
		default:
			return fmt.Errorf("http serve failed:%v", err)
		}
	}
	return nil
}

func (hs *httpServ) Shutdown() {
	close(hs.quit)
	if hs.ln != nil {
		hs.ln.Close()
	}
}

// BindHandler binds skycoin address with a bitcoin address
// Method: GET
// URI: /bind
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
// URI: /status
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
