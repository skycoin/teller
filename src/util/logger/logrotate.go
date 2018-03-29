package logger

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

// Adapted from https://github.com/NYTimes/logrotate
// os.FileMode added

// LogrotateFile wraps an *os.LogrotateFile and listens for a 'SIGHUP' signal from logrotated
// so it can reopen the new file.
type LogrotateFile struct {
	*os.File
	me     sync.Mutex
	path   string
	sighup chan os.Signal
	mode   os.FileMode
}

// NewLogrotateFile creates a LogrotateFile pointer and kicks off the goroutine listening for
// SIGHUP signals.
func NewLogrotateFile(path string, mode os.FileMode) (*LogrotateFile, error) {
	lr := &LogrotateFile{
		me:     sync.Mutex{},
		path:   path,
		sighup: make(chan os.Signal, 1),
		mode:   mode,
	}

	if err := lr.reopen(); err != nil {
		return nil, err
	}

	go func() {
		signal.Notify(lr.sighup, syscall.SIGHUP)

		for range lr.sighup {
			fmt.Fprintf(os.Stderr, "%s: Reopening %q\n", time.Now(), lr.path) // nolint: gas
			if err := lr.reopen(); err != nil {
				fmt.Fprintf(os.Stderr, "%s: Error reopening: %s\n", time.Now(), err) // nolint: gas
			}
		}
	}()

	return lr, nil

}

func (lr *LogrotateFile) reopen() error {
	lr.me.Lock()
	defer lr.me.Unlock()
	lr.File.Close() // nolint: gas,errcheck
	var err error
	lr.File, err = os.OpenFile(lr.path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, lr.mode)
	return err
}

// Write will write to the underlying file. It uses a sync.Mutex to ensure
// uninterrupted writes during logrotates.
func (lr *LogrotateFile) Write(b []byte) (int, error) {
	lr.me.Lock()
	defer lr.me.Unlock()
	return lr.File.Write(b)
}

// Close will stop the goroutine listening for SIGHUP signals and then close
// the underlying os.File.
func (lr *LogrotateFile) Close() error {
	lr.me.Lock()
	defer lr.me.Unlock()
	signal.Stop(lr.sighup)
	close(lr.sighup)
	return lr.File.Close()
}
