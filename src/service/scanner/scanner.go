package scanner

// Scanner provids helper apis to interact with scan service
type Scanner struct {
	s *ScanService
}

// NewScanner creates scanner instance
func NewScanner(s *ScanService) *Scanner {
	return &Scanner{s: s}
}

// AddScanAddress add new address to scan service
func (s *Scanner) AddScanAddress(addr string) error {
	return s.s.AddScanAddress(addr)
}

// GetDepositValue returns deposit value channel
func (s *Scanner) GetDepositValue() <-chan DepositNote {
	return s.s.depositC
}

// GetScanAddresses returns all scanning addresses
func (s *Scanner) GetScanAddresses() []string {
	return s.s.getScanAddresses()
}
