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
			satoshis: 1,
			rate:     "12k",
			err:      errors.New("can't convert 12k to decimal"),
		},
		{
			satoshis: 1,
			rate:     "1b",
			err:      errors.New("can't convert 1b to decimal"),
		},
		{
			satoshis: 1,
			rate:     "",
			err:      errors.New("can't convert  to decimal"),
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
		{
			satoshis: 12345e8, // 12345 BTC
			rate:     "1/2",
			result:   6172e6, // 6172 SKY
		},
		{
			satoshis: 1,
			rate:     "0.0001",
			result:   0, // 0 SKY
		},
		{
			satoshis: 12345678, // 0.12345678 BTC
			rate:     "512",
			result:   63e6, // 63 SKY
		},
		{
			satoshis: 123456789, // 1.23456789 BTC
			rate:     "10000",
			result:   12345e6, // 12345 SKY
		},
		{
			satoshis: 876543219e4, // 87654.3219BTC
			rate:     "2/3",
			result:   58436e6, // 58436 SKY
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
			wei:  big.NewInt(1),
			rate: "100k",
			err:  errors.New("can't convert 100k to decimal"),
		},
		{
			wei:  big.NewInt(1),
			rate: "0.1b",
			err:  errors.New("can't convert 0.1b to decimal"),
		},
		{
			wei:  big.NewInt(1),
			rate: "",
			err:  errors.New("can't convert  to decimal"),
		},

		{
			wei:    big.NewInt(0),
			rate:   "1",
			result: 0,
		},
		{
			wei:    big.NewInt(1000),
			rate:   "0.001",
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
			result: 11e6, // 11 SKY
		},
		{
			wei:    big.NewInt(1).Mul(big.NewInt(2245236), big.NewInt(1e14)), // 224.5236 ETH
			rate:   "200",
			result: 44904e6, // 44904 SKY
		},
		{
			wei:    big.NewInt(1).Mul(big.NewInt(2245236), big.NewInt(1e14)), // 224.5236 ETH
			rate:   "1568",
			result: 352053e6, // 352053(224.5236 * 1568=352053.0048) SKY
		},
		{
			wei:    big.NewInt(1).Mul(big.NewInt(2245236), big.NewInt(1e14)), // 224.5236 ETH
			rate:   "0.15",
			result: 33e6, // 33 SKY
		},
		{
			wei:    big.NewInt(1).Mul(big.NewInt(2245236), big.NewInt(1e14)), // 224.5236 ETH
			rate:   "2/3",
			result: 149e6, // 149 SKY
		},
	}

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
