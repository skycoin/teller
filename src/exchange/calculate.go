package exchange

import (
	"errors"
	"math/big"

	"github.com/shopspring/decimal"

	"github.com/MDLlife/MDL/src/util/droplet"

	"github.com/MDLlife/teller/src/util/mathutil"
)

const (
	// SatoshisPerBTC is the number of satoshis per 1 BTC
	SatoshisPerBTC int64 = 1e8
	// WeiPerETH is the number of wei per 1 ETH
	WeiPerETH int64 = 1e18
)

// CalculateBtcMDLValue returns the amount of MDL (in droplets) to give for an
// amount of BTC (in satoshis).
// Rate is measured in MDL per BTC. It should be a decimal string.
// MaxDecimals is the number of decimal places to truncate to.
func CalculateBtcMDLValue(satoshis int64, mdlPerBTC string, maxDecimals int) (uint64, error) {
	if satoshis < 0 {
		return 0, errors.New("satoshis must be greater than or equal to 0")
	}
	if maxDecimals < 0 {
		return 0, errors.New("maxDecimals can't be negative")
	}

	rate, err := mathutil.ParseRate(mdlPerBTC)
	if err != nil {
		return 0, err
	}

	btc := decimal.New(satoshis, 0)
	btcToSatoshi := decimal.New(SatoshisPerBTC, 0)
	btc = btc.DivRound(btcToSatoshi, 8)

	mdl := btc.Mul(rate)
	mdl = mdl.Truncate(int32(maxDecimals))

	mdlToDroplets := decimal.New(droplet.Multiplier, 0)
	droplets := mdl.Mul(mdlToDroplets)

	amt := droplets.IntPart()
	if amt < 0 {
		// This should never occur, but double check before we convert to uint64,
		// otherwise we would send all the coins due to integer wrapping.
		return 0, errors.New("calculated mdl amount is negative")
	}

	return uint64(amt), nil
}

// CalculateEthMDLValue returns the amount of MDL (in droplets) to give for an
// amount of Eth (in wei).
// Rate is measured in MDL per Eth
func CalculateEthMDLValue(wei *big.Int, mdlPerETH string, maxDecimals int) (uint64, error) {
	if wei.Sign() < 0 {
		return 0, errors.New("wei must be greater than or equal to 0")
	}
	if maxDecimals < 0 {
		return 0, errors.New("maxDecimals can't be negative")
	}
	rate, err := mathutil.ParseRate(mdlPerETH)
	if err != nil {
		return 0, err
	}

	eth := decimal.NewFromBigInt(wei, 0)
	ethToWei := decimal.New(WeiPerETH, 0)
	eth = eth.DivRound(ethToWei, 18)

	mdl := eth.Mul(rate)
	mdl = mdl.Truncate(int32(maxDecimals))

	mdlToDroplets := decimal.New(droplet.Multiplier, 0)
	droplets := mdl.Mul(mdlToDroplets)

	amt := droplets.IntPart()
	if amt < 0 {
		// This should never occur, but double check before we convert to uint64,
		// otherwise we would send all the coins due to integer wrapping.
		return 0, errors.New("calculated mdl amount is negative")
	}

	return uint64(amt), nil
}
