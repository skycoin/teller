package scanner

import (
	"errors"
	"strings"

	"github.com/boltdb/bolt"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/skycoin/teller/src/util/mathutil"
)

// ScanBlock scans a eth block for deposits and adds them
// If the deposit already exists, the result is omitted from the returned list
func (s *ETHStore) ScanBlock(blockInfo interface{}) ([]Deposit, error) {
	var dvs []Deposit
	block, ok := blockInfo.(*types.Block)
	if !ok {
		s.log.Error("convert to types.Block failed")
		return dvs, errors.New("convert to types.Block failed")
	}

	if err := s.db.Update(func(tx *bolt.Tx) error {
		addrs, err := s.getScanAddressesTx(tx)
		if err != nil {
			s.log.WithError(err).Error("getScanAddressesTx failed")
			return err
		}

		deposits, err := ScanETHBlock(block, addrs)
		if err != nil {
			s.log.WithError(err).Error("ScanETHBlock failed")
			return err
		}

		for _, dv := range deposits {
			if err := s.pushDepositTx(tx, dv); err != nil {
				log := s.log.WithField("deposit", dv)
				switch err.(type) {
				case DepositExistsErr:
					log.Warning("Deposit already exists in db")
					continue
				default:
					log.WithError(err).Error("pushDepositTx failed")
					return err
				}
			}

			dvs = append(dvs, dv)
		}

		return nil
	}); err != nil {
		return nil, err
	}

	return dvs, nil
}

// ScanETHBlock scan the given block and returns the next block hash or error
func ScanETHBlock(block *types.Block, depositAddrs []string) ([]Deposit, error) {
	addrMap := map[string]struct{}{}
	for _, a := range depositAddrs {
		addrMap[a] = struct{}{}
	}

	var dv []Deposit
	for i, tx := range block.Transactions() {
		to := tx.To()
		if to == nil {
			//this is a contract transcation
			continue
		}
		//1 eth = 1e18 wei ,tx.Value() is very big that may overflow(int64), so store it as Gwei(1Gwei=1e9wei) and recover it when used
		amt := mathutil.Wei2Gwei(tx.Value())
		a := strings.ToLower(to.String())
		if _, ok := addrMap[a]; ok {
			dv = append(dv, Deposit{
				CoinType: CoinTypeETH,
				Address:  a,
				Value:    amt,
				Height:   int64(block.NumberU64()),
				Tx:       tx.Hash().String(),
				N:        uint32(i),
			})
		}
	}

	return dv, nil
}
