package scanner

import (
	"errors"
	"fmt"
	"sync"
)

type Multiplexer struct {
	scannerMap   map[string]Scanner
	outChan      chan DepositNote
	scannerCount int
	sync.RWMutex
}

func NewMultiplexer() *Multiplexer {
	return &Multiplexer{
		scannerMap:   map[string]Scanner{},
		outChan:      make(chan DepositNote, 1000),
		scannerCount: 0,
	}
}

//AddScanner add scanner of coinType
func (m *Multiplexer) AddScanner(scanner Scanner, coinType string) error {
	if scanner == nil {
		return errors.New(fmt.Sprintf("nil scanner of coinType %s", coinType))
	}
	m.RWMutex.Lock()
	defer m.RWMutex.Unlock()
	_, existsScanner := m.scannerMap[coinType]
	if existsScanner {
		return errors.New(fmt.Sprintf("scanner of coinType %s already exists", coinType))
	}

	m.scannerMap[coinType] = scanner
	m.scannerCount++
	return nil
}

func (m *Multiplexer) AddScanAddress(depositAddr, coinType string) error {
	m.RWMutex.Lock()
	defer m.RWMutex.Unlock()
	scanner, existsScanner := m.scannerMap[coinType]
	if !existsScanner {
		return errors.New("unknown cointype")
	}
	scanner.AddScanAddress(depositAddr)
	return nil
}

//Multiplex forward multi-scanner deposit to a shared aggregate channel
func (m *Multiplexer) Multiplex() error {
	for _, scan := range m.scannerMap {
		go func(scan Scanner) {
			for dv := range scan.GetDeposit() {
				m.outChan <- dv
			}
		}(scan)
	}

	return nil
}

func (m *Multiplexer) GetDeposits() <-chan DepositNote {
	return m.outChan
}

func (m *Multiplexer) GetScannerCount() int {
	return m.scannerCount
}

func (m *Multiplexer) Shudown() {
}
