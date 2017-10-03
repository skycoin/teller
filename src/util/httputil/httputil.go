package httputil

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/skycoin/teller/src/util/logger"
)

// ErrResponse write error message and code
func ErrResponse(w http.ResponseWriter, code int, errMsg ...string) {
	if len(errMsg) > 0 {
		http.Error(w, strings.Join(errMsg, " "), code)
	} else {
		http.Error(w, http.StatusText(code), code)
	}
}

// JSONResponse marshal data into json and write response
func JSONResponse(w http.ResponseWriter, data interface{}) error {
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

// LogHandler log middleware
func LogHandler(log logger.Logger, hd http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := time.Now()
		hd(w, r)
		log.Printf("HTTP [%s] %dms %s \n", r.Method, time.Since(t)/time.Millisecond, r.URL.String())
	}
}
