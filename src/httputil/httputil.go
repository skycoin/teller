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

	if _, err := w.Write(d); err != nil {
		return err
	}
	return nil
}

// LogHandler log middleware
func LogHandler(log *logrus.Logger, hd http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := logger.WithContext(r.Context(), log)
		r = r.WithContext(ctx)

		t := time.Now()

		hd(w, r)

		log.WithFields(logrus.Fields{
			"method":   r.Method,
			"duration": fmt.Sprintf("%dms", time.Since(t)/time.Millisecond),
			"url":      r.URL.String(),
		}).Info("HTTP Request")
	}
}
