package scanner

import (
	"errors"
	"fmt"
	"sync"

	"github.com/sirupsen/logrus"
)

// Multiplexer manager of scanner
type Multiplexer struct {
	scannerMap   map[string]Scanner
	outChan      chan DepositNote
	scannerCount int
	quit         chan struct{}
	done         chan struct{}
	log          logrus.FieldLogger
	sync.RWMutex
}

// NewMultiplexer create multiplexer instance
func NewMultiplexer(log logrus.FieldLogger) *Multiplexer {
	return &Multiplexer{
		scannerMap:   map[string]Scanner{},
		outChan:      make(chan DepositNote, 1000),
		scannerCount: 0,
		log:          log.WithField("prefix", "scanner.multiplex"),
		quit:         make(chan struct{}),
		done:         make(chan struct{}, 1),
	}
}

// AddScanner add scanner of coinType
func (m *Multiplexer) AddScanner(scanner Scanner, coinType string) error {
	if scanner == nil {
		return errors.New("nil scanner")
	}
	m.RWMutex.Lock()
	defer m.RWMutex.Unlock()
	_, existsScanner := m.scannerMap[coinType]
	if existsScanner {
		return fmt.Errorf("scanner of coinType %s already exists", coinType)
	}

	m.scannerMap[coinType] = scanner
	m.scannerCount++
	return nil
}

// AddScanAddress adds new scan address to scanner according to coinType
func (m *Multiplexer) AddScanAddress(depositAddr, coinType string) error {
	m.RWMutex.Lock()
	defer m.RWMutex.Unlock()

	scanner, ok := m.scannerMap[coinType]
	if !ok {
		return fmt.Errorf("unknown cointype \"%s\"", coinType)
	}

	return scanner.AddScanAddress(depositAddr, coinType)
}

// ValidateCoinType returns an error if the coinType is invalid
func (m *Multiplexer) ValidateCoinType(coinType string) error {
	m.RWMutex.RLock()
	defer m.RWMutex.RUnlock()

	if _, ok := m.scannerMap[coinType]; !ok {
		return fmt.Errorf("unknown cointype \"%s\"", coinType)
	}

	return nil
}

// Multiplex forward multi-scanner deposit to a shared aggregate channel, think of "Goroutine merging channel"
func (m *Multiplexer) Multiplex() error {
	log := m.log.WithField("scanner-count", m.scannerCount)
	log.Info("Start multiplex service")
	defer func() {
		log.Info("Multiplex service closed")
		close(m.done)
	}()

	var wg sync.WaitGroup
	for scannerName, scan := range m.scannerMap {
		wg.Add(1)
		go func(name string, scan Scanner) {
			defer log.Info("Scan goroutine exited")
			defer wg.Done()
			for {
				select {
				case dv, ok := <-scan.GetDeposit():
					if !ok {
						log.WithField("name", name).Info("sub-scanner closed")
						return
					}
					m.outChan <- dv
				case <-m.quit:
					return
				}
			}
		}(scannerName, scan)
	}
	wg.Wait()

	return nil
}

// Shutdown shutdown the multiplexer
func (m *Multiplexer) Shutdown() {
	m.log.Info("Closing Multiplexer")
	close(m.quit)
	close(m.outChan)
	m.log.Info("Waiting for Multiplexer to stop")
	<-m.done
}

// GetDeposit returns deposit values channel.
func (m *Multiplexer) GetDeposit() <-chan DepositNote {
	return m.outChan
}

// GetScannerCount returns scanner count.
func (m *Multiplexer) GetScannerCount() int {
	return m.scannerCount
}

// GetScanner returns Scanner according to coinType
func (m *Multiplexer) GetScanner(coinType string) Scanner {
	scanner, existsScanner := m.scannerMap[coinType]
	if !existsScanner {
		return nil
	}
	return scanner
}
