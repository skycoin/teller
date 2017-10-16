package exchange

import (
	"errors"

	"github.com/shopspring/decimal"

	"github.com/skycoin/skycoin/src/daemon"
	"github.com/skycoin/skycoin/src/util/droplet"
)

// calculateSkyValue returns the amount of SKY (in droplets) to give for an
// amount of BTC (in satoshis).
// Rate is measured in SKY per BTC.
func calculateSkyValue(satoshis, skyPerBTC int64) (uint64, error) {
	if satoshis < 0 || skyPerBTC < 0 {
		return 0, errors.New("negative satoshis or negative skyPerBTC")
	}

	btc := decimal.New(satoshis, 0)
	btcToSatoshi := decimal.New(satoshiPerBTC, 0)
	btc = btc.DivRound(btcToSatoshi, 8)

	rate := decimal.New(skyPerBTC, 0)

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
