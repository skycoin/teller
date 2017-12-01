package exchange

import (
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCalculateSkyValue(t *testing.T) {
	cases := []struct {
		satoshis int64
		rate     string
		result   uint64
		err      error
	}{
		{
			satoshis: -1,
			rate:     "1",
			err:      errors.New("satoshis must be greater than or equal to 0"),
		},

		{
			satoshis: 1,
			rate:     "-1",
			err:      errors.New("rate must be greater than zero"),
		},

		{
			satoshis: 1,
			rate:     "0",
			err:      errors.New("rate must be greater than zero"),
		},

		{
			satoshis: 1,
			rate:     "invalidrate",
			err:      errors.New("can't convert invalidrate to decimal: exponent is not numeric"),
		},

		{
			satoshis: 0,
			rate:     "1",
			result:   0,
		},

		{
			satoshis: 1e8,
			rate:     "1",
			result:   1e6,
		},

		{
			satoshis: 1e8,
			rate:     "500",
			result:   500e6,
		},

		{
			satoshis: 100e8,
			rate:     "500",
			result:   50000e6,
		},

		{
			satoshis: 2e5,   // 0.002 BTC
			rate:     "500", // 500 SKY/BTC = 1 SKY / 0.002 BTC
			result:   1e6,   // 1 SKY
		},

		{
			satoshis: 1e8, // 1 BTC
			rate:     "1/2",
			result:   0, // 0.5 SKY
		},
	}

	for _, tc := range cases {
		name := fmt.Sprintf("satoshis=%d rate=%s", tc.satoshis, tc.rate)
		t.Run(name, func(t *testing.T) {
			result, err := CalculateBtcSkyValue(tc.satoshis, tc.rate)
			if tc.err == nil {
				require.NoError(t, err)
				require.Equal(t, tc.result, result, "%d != %d", tc.result, result)
			} else {
				require.Error(t, err)
				require.Equal(t, tc.err, err)
				require.Equal(t, uint64(0), result, "%d != 0", result)
			}
		})
	}
}
func TestCalculateEthSkyValue(t *testing.T) {
	cases := []struct {
		wei    *big.Int
		rate   string
		result uint64
		err    error
	}{
		{
			wei:  big.NewInt(-1),
			rate: "1",
			err:  errors.New("wei must be greater than or equal to 0"),
		},

		{
			wei:  big.NewInt(1),
			rate: "-1",
			err:  errors.New("rate must be greater than zero"),
		},

		{
			wei:  big.NewInt(1),
			rate: "0",
			err:  errors.New("rate must be greater than zero"),
		},

		{
			wei:  big.NewInt(1),
			rate: "invalidrate",
			err:  errors.New("can't convert invalidrate to decimal: exponent is not numeric"),
		},

		{
			wei:    big.NewInt(0),
			rate:   "1",
			result: 0,
		},

		{
			wei:    big.NewInt(1e18),
			rate:   "1",
			result: 1e6,
		},

		{
			wei:    big.NewInt(1e18),
			rate:   "500",
			result: 500e6,
		},

		{
			wei:    big.NewInt(1).Mul(big.NewInt(100), big.NewInt(1e18)),
			rate:   "500",
			result: 50000e6,
		},

		{
			wei:    big.NewInt(2e15), // 0.002 ETH
			rate:   "500",            // 500 SKY/ETH = 1 SKY / 0.002 ETH
			result: 1e6,              // 1 SKY
		},

		{
			wei:    big.NewInt(1e18), // 1 ETH
			rate:   "1/2",
			result: 0, // 0.5 SKY
		},

		{
			wei:    big.NewInt(11345e13), // 0.11345 ETH
			rate:   "100",
			result: 11000000, // 11 SKY
		},
	}
	testEth := big.NewInt(1).Mul(big.NewInt(1e18), big.NewInt(1e3)) //1e21 1000ETH
	bigNum1 := big.NewInt(1).Div(testEth, big.NewInt(1e8))          //1e13
	midDepositValue := bigNum1.Int64()                              //1e13
	require.Equal(t, midDepositValue, big.NewInt(1e13).Int64())
	originEth := big.NewInt(1).Mul(big.NewInt(midDepositValue), big.NewInt(1e8)) //1e21
	require.Equal(t, 0, originEth.Cmp(testEth))
	droplets := int64(2240200000)
	amt := big.NewInt(1).Div(big.NewInt(droplets), big.NewInt(1000000)).Int64() * int64(1000000)
	require.Equal(t, amt, int64(2240000000))

	for _, tc := range cases {
		name := fmt.Sprintf("wei=%d rate=%s", tc.wei, tc.rate)
		t.Run(name, func(t *testing.T) {
			result, err := CalculateEthSkyValue(tc.wei, tc.rate)
			if tc.err == nil {
				require.NoError(t, err)
				require.Equal(t, tc.result, result, "%d != %d", tc.result, result)
			} else {
				require.Error(t, err)
				require.Equal(t, tc.err, err)
				require.Equal(t, uint64(0), result, "%d != 0", result)
			}
		})
	}
}
