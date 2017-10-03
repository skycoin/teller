package logger

import (
	"fmt"
	"io"
	"log"
	"os"
)

// A Logger is similar to log.Logger with Debug(ln|f)? methods
type Logger interface {
	// SetDebug is used to change debug logs flag. The method is not
	// safe for async usage
	SetDebug(bool)

	SetPrefix(string)    //
	SetFlags(int)        //
	SetOutput(io.Writer) //

	Print(...interface{})          //
	Println(...interface{})        //
	Printf(string, ...interface{}) //

	Panic(...interface{})          //
	Panicln(...interface{})        //
	Panicf(string, ...interface{}) //

	Fatal(...interface{})          //
	Fatalln(...interface{})        //
	Fatalf(string, ...interface{}) //

	Debug(...interface{})          //
	Debugln(...interface{})        //
	Debugf(string, ...interface{}) //
}

type logger struct {
	*log.Logger
	debug bool
}

// NewLogger create new Logger with given prefix and debug-enabling value.
// By default flags of the Logger is log.Lshortfile|log.Ltime
func NewLogger(prefix string, debug bool, multiW ...io.Writer) Logger {
	mw := io.MultiWriter(append(multiW, os.Stdout)...)
	return &logger{
		Logger: log.New(mw, prefix, log.Lshortfile|log.Ltime|log.Ldate),
		debug:  debug,
	}
}

func (l *logger) Debug(args ...interface{}) {
	if l.debug {
		l.Output(2, fmt.Sprint(args...))
	}
}

func (l *logger) Debugln(args ...interface{}) {
	if l.debug {
		l.Output(2, fmt.Sprintln(args...))
	}
}

func (l *logger) Debugf(format string, args ...interface{}) {
	if l.debug {
		l.Output(2, fmt.Sprintf(format, args...))
	}
}

func (l *logger) SetDebug(debug bool) {
	l.debug = debug
}
