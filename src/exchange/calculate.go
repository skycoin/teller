package exchange

import (
	"errors"

	"github.com/shopspring/decimal"

	"github.com/skycoin/teller/src/util/mathutil"

	"github.com/skycoin/skycoin/src/daemon"
	"github.com/skycoin/skycoin/src/util/droplet"
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

	amt := droplets.IntPart()
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
