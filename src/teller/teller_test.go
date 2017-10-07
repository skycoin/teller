package teller

import (
	"github.com/skycoin/teller/src/teller/exchange"
)

type dummyExchanger struct {
	err      error
	skyAddrs map[string][]string
}

func (de dummyExchanger) BindAddress(btcAddr, skyAddr string) error {
	if de.err != nil {
		return de.err
	}

	if de.skyAddrs == nil {
		de.skyAddrs = make(map[string][]string)
	}

	btcAddrs := de.skyAddrs[skyAddr]
	if btcAddrs == nil {
		btcAddrs = []string{}
	}

	btcAddrs = append(btcAddrs, btcAddr)
	de.skyAddrs[skyAddr] = btcAddrs

	return de.err
}

func (de dummyExchanger) GetDepositStatuses(skyAddr string) ([]exchange.DepositStatus, error) {
	return nil, nil
}

func (de dummyExchanger) BindNum(skyAddr string) (int, error) {
	if de.skyAddrs == nil {
		return 0, nil
	}

	return len(de.skyAddrs[skyAddr]), nil
}

type dummyBtcAddrGenerator struct {
	addr string
	err  error
}

func (dba dummyBtcAddrGenerator) NewAddress() (string, error) {
	return dba.addr, dba.err
}
