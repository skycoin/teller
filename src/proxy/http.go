package proxy

import (
	"context"
	"fmt"
	"net/http"

	"strings"

	"encoding/json"

	"time"

	"strconv"

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
)

var httpErrCodeStr = []string{
	errBadRequest:       "Bad Request",
	errMethodNotAllowed: "Method Not Allowed",
	errNotAcceptable:    "Not Acceptable",
	errInternalServErr:  "Internal Server Error",
	errRequestTimeout:   "Request Timeout",
}

type httpServ struct {
	srvAddr string
	log     logger.Logger
	gateway *gateway
}

// creates a http service
func newHTTPServ(addr string, log logger.Logger, gw *gateway) *httpServ {
	return &httpServ{srvAddr: addr, log: log, gateway: gw}
}

func (hs *httpServ) Run(cxt context.Context) {
	hs.log.Println("Http service start, serve on", hs.srvAddr)
	mux := http.NewServeMux()
	mux.HandleFunc("/monitor", MonitorHandler(hs.gateway))
	mux.HandleFunc("/exchange_logs", ExchangeLogsHandler(hs.gateway))

	errC := make(chan error, 1)
	go func() {
		errC <- http.ListenAndServe(hs.srvAddr, mux)
	}()

	select {
	case <-cxt.Done():
		return
	case err := <-errC:
		hs.log.Debugln(err)
	}
	hs.log.Debugln("http service exit")
}

// MonitorHandler monitor handler function
// Method: POST
// URI: /monitor
// Content-Type: application/json
func MonitorHandler(gw gatewayer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.Header().Set("Allow", "POST")
			errorResponse(w, http.StatusMethodNotAllowed)
			gw.Log().Println(httpErrCodeStr[http.StatusMethodNotAllowed])
			return
		}

		ct := r.Header.Get("Content-Type")
		if !strings.Contains(ct, "application/json") {
			w.Header().Set("Accept", "application/json")
			errorResponse(w, http.StatusNotAcceptable)
			gw.Log().Println(httpErrCodeStr[http.StatusNotAcceptable])
			return
		}

		var mm daemon.MonitorMessage
		if err := json.NewDecoder(r.Body).Decode(&mm); err != nil {
			errorResponse(w, http.StatusBadRequest)
			gw.Log().Println(httpErrCodeStr[http.StatusBadRequest])
			return
		}

		cxt, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		ack, err := gw.AddMonitor(cxt, &mm)
		if err != nil {
			if err == context.DeadlineExceeded {
				errorResponse(w, http.StatusRequestTimeout)
				gw.Log().Println(httpErrCodeStr[http.StatusRequestTimeout])
				return
			}
			errorResponse(w, http.StatusInternalServerError)
			gw.Log().Println(err)
			return
		}

		if err := jsonResponse(w, ack); err != nil {
			gw.Log().Println(err)
		}
	}
}

// ExchangeLogsHandler api for querying exchange logs.
// Method: GET
// URI: /exchange_logs?start=$start&&end=$end
func ExchangeLogsHandler(gw gatewayer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.Header().Set("Allow", "GET")
			errorResponse(w, http.StatusMethodNotAllowed)
			gw.Log().Println(httpErrCodeStr[http.StatusMethodNotAllowed])
			return
		}

		startStr := r.URL.Query().Get("start")
		if startStr == "" {
			errStr := "Missing 'start' param in url"
			http.Error(w, errStr, http.StatusBadRequest)
			gw.Log().Println(errStr)
			return
		}

		start, err := strconv.Atoi(startStr)
		if err != nil {
			errStr := fmt.Sprintf("Invalid 'start' param, err:%v", err)
			http.Error(w, errStr, http.StatusBadRequest)
			gw.Log().Println(errStr)
			return
		}

		endStr := r.URL.Query().Get("end")
		if endStr == "" {
			errStr := "Missing 'end' param in url"
			http.Error(w, errStr, http.StatusBadRequest)
			gw.Log().Println(errStr)
			return
		}

		end, err := strconv.Atoi(endStr)
		if err != nil {
			errStr := fmt.Sprintf("Invalid 'end' param, err:%v", err)
			http.Error(w, errStr, http.StatusBadRequest)
			gw.Log().Println(errStr)
			return
		}

		if start > end {
			errStr := fmt.Sprintf("start must >= end")
			http.Error(w, errStr, http.StatusBadRequest)
			gw.Log().Println(errStr)
			return
		}

		// divide the start end id into small block, so that the response won't be too big
		idPairs := divideStartEndRange(start, end, maxRequestLogsNum)
		totalLogs := []daemon.ExchangeLog{}
		var maxLogsID int
		for _, pair := range idPairs {
			if maxLogsID != 0 && maxLogsID < pair[0] {
				break
			}

			msg := &daemon.GetExchangeLogsMessage{
				StartID: pair[0],
				EndID:   pair[1],
			}

			cxt, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			ack, err := gw.GetExchangeLogs(cxt, msg)
			if err != nil {
				if err == context.DeadlineExceeded {
					errorResponse(w, http.StatusRequestTimeout)
					gw.Log().Println(httpErrCodeStr[http.StatusRequestTimeout])
					return
				}
				errorResponse(w, http.StatusInternalServerError)
				gw.Log().Println(err)
				return
			}
			totalLogs = append(totalLogs, ack.Logs...)
			maxLogsID = ack.MaxLogID
		}
		ack := daemon.GetExchangeLogsAckMessage{
			Result: daemon.Result{
				Success: true,
			},
			MaxLogID: maxLogsID,
			Logs:     totalLogs,
		}

		if err := jsonResponse(w, ack); err != nil {
			gw.Log().Println(err)
		}
	}
}

func divideStartEndRange(start, end, blocksize int) [][]int {
	if start > end {
		return [][]int{}
	}

	if blocksize == 0 {
		return [][]int{{start, end}}
	}

	if end-start+1 <= blocksize {
		return [][]int{
			[]int{start, end},
		}
	}

	n := (end - start + 1) / blocksize

	pairs := make([][]int, 0, n)
	for i := 0; i < n; i++ {
		pairs = append(pairs, []int{start + i*blocksize, start - 1 + (i+1)*blocksize})
	}

	if ((end - start + 1) % blocksize) != 0 {
		pairs = append(pairs, []int{start + n*blocksize, end})
	}
	return pairs
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
