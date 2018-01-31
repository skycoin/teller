package exchange

import (
	"errors"
	"math/big"

	"github.com/shopspring/decimal"

	"github.com/skycoin/skycoin/src/util/droplet"

	"github.com/skycoin/teller/src/util/mathutil"
)

const (
	// SatoshisPerBTC is the number of satoshis per 1 BTC
	SatoshisPerBTC int64 = 1e8
	// WeiPerETH is the number of wei per 1 ETH
	WeiPerETH int64 = 1e18
)

// CalculateBtcSkyValue returns the amount of SKY (in droplets) to give for an
// amount of BTC (in satoshis).
// Rate is measured in SKY per BTC. It should be a decimal string.
// MaxDecimals is the number of decimal places to truncate to.
func CalculateBtcSkyValue(satoshis int64, skyPerBTC string, maxDecimals int) (uint64, error) {
	if satoshis < 0 {
		return 0, errors.New("satoshis must be greater than or equal to 0")
	}
	if maxDecimals < 0 {
		return 0, errors.New("maxDecimals can't be negative")
	}

	rate, err := mathutil.ParseRate(skyPerBTC)
	if err != nil {
		return 0, err
	}

	btc := decimal.New(satoshis, 0)
	btcToSatoshi := decimal.New(SatoshisPerBTC, 0)
	btc = btc.DivRound(btcToSatoshi, 8)

	sky := btc.Mul(rate)
	sky = sky.Truncate(int32(maxDecimals))

	skyToDroplets := decimal.New(droplet.Multiplier, 0)
	droplets := sky.Mul(skyToDroplets)

	amt := droplets.IntPart()
	if amt < 0 {
		// This should never occur, but double check before we convert to uint64,
		// otherwise we would send all the coins due to integer wrapping.
		return 0, errors.New("calculated sky amount is negative")
	}

	return uint64(amt), nil
}

// CalculateEthSkyValue returns the amount of SKY (in droplets) to give for an
// amount of Eth (in wei).
// Rate is measured in SKY per Eth
func CalculateEthSkyValue(wei *big.Int, skyPerETH string, maxDecimals int) (uint64, error) {
	if wei.Sign() < 0 {
		return 0, errors.New("wei must be greater than or equal to 0")
	}
	if maxDecimals < 0 {
		return 0, errors.New("maxDecimals can't be negative")
	}
	rate, err := mathutil.ParseRate(skyPerETH)
	if err != nil {
		return 0, err
	}

	eth := decimal.NewFromBigInt(wei, 0)
	ethToWei := decimal.New(WeiPerETH, 0)
	eth = eth.DivRound(ethToWei, 18)

	sky := eth.Mul(rate)
	sky = sky.Truncate(int32(maxDecimals))

	skyToDroplets := decimal.New(droplet.Multiplier, 0)
	droplets := sky.Mul(skyToDroplets)

	amt := droplets.IntPart()
	if amt < 0 {
		// This should never occur, but double check before we convert to uint64,
		// otherwise we would send all the coins due to integer wrapping.
		return 0, errors.New("calculated sky amount is negative")
	}

	return uint64(amt), nil
}
