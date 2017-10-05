package httputil

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/skycoin/teller/src/logger"
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

	_, err = w.Write(d)
	return err
}

// LogHandler log middleware
func LogHandler(log logrus.FieldLogger, hd http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log = log.WithFields(logrus.Fields{
			"method":     r.Method,
			"remoteAddr": r.RemoteAddr,
			"url":        r.URL.String(),
		})
		ctx = logger.WithContext(ctx, log)
		r = r.WithContext(ctx)

		t := time.Now()

		hd(w, r)

		log.WithFields(logrus.Fields{
			"duration": fmt.Sprintf("%dms", time.Since(t)/time.Millisecond),
		}).Info("HTTP Request")
	}
}
