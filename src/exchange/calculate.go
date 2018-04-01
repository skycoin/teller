package exchange

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/shopspring/decimal"

	"github.com/skycoin/skycoin/src/util/droplet"

	"github.com/skycoin/teller/src/config"
	"github.com/skycoin/teller/src/util/mathutil"
)

const (
	// SatoshiExponent is the number of decimal places for 1 satoshi
	SatoshiExponent int64 = 8
	// SatoshisPerBTC is the number of satoshis per 1 BTC
	SatoshisPerBTC int64 = 1e8

	// WeiExponent is the number of decimal places for 1 wei
	WeiExponent int64 = 18
	// WeiPerETH is the number of wei per 1 ETH
	WeiPerETH int64 = 1e18
)

func init() {
	for _, k := range config.CoinTypes {
		_, err := DepositAmountToString(k, 0)
		if err != nil {
			panic(fmt.Sprintf("DepositAmountToString switch is missing a case for CoinType %s", k))
		}
	}
}

// DepositAmountToString converts a deposit amount of a given coin type to its fixed string representation
func DepositAmountToString(coinType string, amount int64) (string, error) {
	switch coinType {
	case config.CoinTypeBTC:
		return BtcAmountToString(amount), nil
	case config.CoinTypeETH:
		return EthAmountToString(amount), nil
	case config.CoinTypeSKY:
		return SkyAmountToString(amount)
	default:
		return "", config.ErrUnsupportedCoinType
	}
}

// BtcAmountToString convert a BTC deposit amount to its fixed string representation
func BtcAmountToString(v int64) string {
	return decimal.New(v, -int32(SatoshiExponent)).StringFixed(int32(SatoshiExponent))
}

// EthAmountToString convert an ETH deposit amount to its fixed string representation
func EthAmountToString(v int64) string {
	return decimal.New(v, -int32(WeiExponent)).StringFixed(int32(WeiExponent))
}

// SkyAmountToString convert a SKY deposit amount to its fixed string representation
func SkyAmountToString(v int64) (string, error) {
	if v < 0 {
		return "", errors.New("sky amount is negative")
	}

	return droplet.ToString(uint64(v))
}

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
