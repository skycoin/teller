package exchange

// Exchange provides helper apis to interact with exchange service
type Exchange struct {
	s *Service
}

// NewExchange creates exchange client
func NewExchange(s *Service) *Exchange {
	return &Exchange{s: s}
}

// BindAddress binds deposit btc address with skycoin address, and
// add the btc address to scan service, when detect deposit coin
// to the btc address, will send specific skycoin to the binded
// skycoin address
func (ec *Exchange) BindAddress(btcAddr, skyAddr string) error {
	return ec.s.addDepositInfo(btcAddr, skyAddr)
}
