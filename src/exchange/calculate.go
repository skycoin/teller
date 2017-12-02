package exchange

import (
	"errors"
	"math/big"

	"github.com/shopspring/decimal"

	"github.com/skycoin/skycoin/src/daemon"
	"github.com/skycoin/skycoin/src/util/droplet"

	"github.com/skycoin/teller/src/util/mathutil"
)

// CalculateBtcSkyValue returns the amount of SKY (in droplets) to give for an
// amount of BTC (in satoshis).
// Rate is measured in SKY per BTC. It should be a decimal string
func CalculateBtcSkyValue(satoshis int64, skyPerBTC string) (uint64, error) {
	if satoshis < 0 {
		return 0, errors.New("satoshis must be greater than or equal to 0")
	}

	rate, err := ParseRate(skyPerBTC)
	if err != nil {
		return 0, err
	}

	btc := decimal.New(satoshis, 0)
	btcToSatoshi := decimal.New(SatoshisPerBTC, 0)
	btc = btc.DivRound(btcToSatoshi, 8)

	sky := btc.Mul(rate)
	sky = sky.Truncate(daemon.MaxDropletPrecision)

	skyToDroplets := decimal.New(droplet.Multiplier, 0)
	droplets := sky.Mul(skyToDroplets)

	amt := big.NewInt(1).Div(big.NewInt(droplets.IntPart()), big.NewInt(1000000)).Uint64() * 1000000
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
func CalculateEthSkyValue(wei *big.Int, skyPerETH string) (uint64, error) {
	if wei.Sign() < 0 {
		return 0, errors.New("wei must be greater than or equal to 0")
	}
	rate, err := ParseRate(skyPerETH)
	if err != nil {
		return 0, err
	}

	eth := decimal.NewFromBigInt(wei, 0)
	ethToWei := decimal.New(WeiPerETH, 0)
	eth = eth.DivRound(ethToWei, 18)

	sky := eth.Mul(rate)
	sky = sky.Truncate(daemon.MaxDropletPrecision)

	skyToDroplets := decimal.New(droplet.Multiplier, 0)
	droplets := sky.Mul(skyToDroplets)

	// floor (x / e6) * e6
	divisor := decimal.New(daemon.MaxDropletDivisor, 0)
	dropletsInt := droplets.DivRound(divisor, 6).IntPart()

	amt := dropletsInt * daemon.MaxDropletDivisor
	if amt < 0 {
		// This should never occur, but double check before we convert to uint64,
		// otherwise we would send all the coins due to integer wrapping.
		return 0, errors.New("calculated sky amount is negative")
	}

	return uint64(amt), nil
}

// ParseRate parses an exchange rate string and validates it
func ParseRate(rate string) (decimal.Decimal, error) {
	r, err := mathutil.DecimalFromString(rate)
	if err != nil {
		return decimal.Decimal{}, err
	}

	if r.LessThanOrEqual(decimal.New(0, 0)) {
		return decimal.Decimal{}, errors.New("rate must be greater than zero")
	}

	return r, nil
}
