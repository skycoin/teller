package logger

import (
	"context"
	"io"
	"os"

	"github.com/sirupsen/logrus"
	prefixed "github.com/x-cray/logrus-prefixed-formatter"
)

// NewLogger creates a logrus.Logger, which logs to os.Stdout.
// If debug is true, the log level is logrus.DebugLevel, otherwise logrus.InfoLevel.
// If logFilename is not the empty string, logs will also be written to that file,
// in addition to os.Stdout.
func NewLogger(logFilename string, debug bool) (*logrus.Logger, error) {
	var out io.Writer = os.Stdout

	if logFilename != "" {
		logFile, err := os.OpenFile(logFilename, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
		if err != nil {
			return nil, err
		}

		out = io.MultiWriter(os.Stdout, logFile)
	}

	level := logrus.InfoLevel
	if debug {
		level = logrus.DebugLevel
	}

	return &logrus.Logger{
		Out: out,
		Formatter: &prefixed.TextFormatter{
			FullTimestamp: true,
		},
		Level: level,
	}, nil
}

type key int

const loggerCtxKey key = 0

// FromContext return a *logrus.Logger from a context
func FromContext(ctx context.Context) *logrus.Logger {
	lg := ctx.Value(loggerCtxKey)
	ruslogger, ok := lg.(*logrus.Logger)
	if !ok {
		return nil
	}
	return ruslogger
}

// WithContext puts a *logrus.Logger into a context
func WithContext(ctx context.Context, lg *logrus.Logger) context.Context {
	return context.WithValue(ctx, loggerCtxKey, lg)
}
