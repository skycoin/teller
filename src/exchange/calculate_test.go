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
		maxDecimals int
		satoshis    int64
		rate        string
		result      uint64
		err         error
	}{
		{
			maxDecimals: 0,
			satoshis:    -1,
			rate:        "1",
			err:         errors.New("satoshis must be greater than or equal to 0"),
		},

		{
			maxDecimals: 0,
			satoshis:    1,
			rate:        "-1",
			err:         errors.New("rate must be greater than zero"),
		},

		{
			maxDecimals: 0,
			satoshis:    1,
			rate:        "0",
			err:         errors.New("rate must be greater than zero"),
		},

		{
			maxDecimals: 0,
			satoshis:    1,
			rate:        "invalidrate",
			err:         errors.New("can't convert invalidrate to decimal: exponent is not numeric"),
		},
		{
			maxDecimals: 0,
			satoshis:    1,
			rate:        "12k",
			err:         errors.New("can't convert 12k to decimal"),
		},
		{
			maxDecimals: 0,
			satoshis:    1,
			rate:        "1b",
			err:         errors.New("can't convert 1b to decimal"),
		},
		{
			maxDecimals: 0,
			satoshis:    1,
			rate:        "",
			err:         errors.New("can't convert  to decimal"),
		},

		{
			maxDecimals: 0,
			satoshis:    0,
			rate:        "1",
			result:      0,
		},

		{
			maxDecimals: 0,
			satoshis:    1e8,
			rate:        "1",
			result:      1e6,
		},

		{
			maxDecimals: 0,
			satoshis:    1e8,
			rate:        "500",
			result:      500e6,
		},

		{
			maxDecimals: 0,
			satoshis:    100e8,
			rate:        "500",
			result:      50000e6,
		},

		{
			maxDecimals: 0,
			satoshis:    2e5,   // 0.002 BTC
			rate:        "500", // 500 SKY/BTC = 1 SKY / 0.002 BTC
			result:      1e6,   // 1 SKY
		},

		{
			maxDecimals: 0,
			satoshis:    1e8, // 1 BTC
			rate:        "1/2",
			result:      0, // 0.5 SKY
		},
		{
			maxDecimals: 0,
			satoshis:    12345e8, // 12345 BTC
			rate:        "1/2",
			result:      6172e6, // 6172 SKY
		},
		{
			maxDecimals: 0,
			satoshis:    1e8,
			rate:        "0.0001",
			result:      0, // 0 SKY
		},
		{
			maxDecimals: 0,
			satoshis:    12345678, // 0.12345678 BTC
			rate:        "512",
			result:      63e6, // 63 SKY
		},
		{
			maxDecimals: 0,
			satoshis:    123456789, // 1.23456789 BTC
			rate:        "10000",
			result:      12345e6, // 12345 SKY
		},
		{
			maxDecimals: 0,
			satoshis:    876543219e4, // 87654.3219 BTC
			rate:        "2/3",
			result:      58436e6, // 58436 SKY
		},

		{
			maxDecimals: 1,
			satoshis:    1e8, // 1 BTC
			rate:        "1/2",
			result:      5e5, // 0.5 SKY
		},
		{
			maxDecimals: 1,
			satoshis:    12345e8, // 12345 BTC
			rate:        "1/2",
			result:      6172e6 + 5e5, // 6172.5 SKY
		},
		{
			maxDecimals: 1,
			satoshis:    1e8,
			rate:        "0.0001",
			result:      0, // 0 SKY
		},
		{
			maxDecimals: 1,
			satoshis:    12345678, // 0.12345678 BTC
			rate:        "512",
			result:      63e6 + 2e5, // 63.2 SKY
		},
		{
			maxDecimals: 1,
			satoshis:    123456789, // 1.23456789 BTC
			rate:        "10000",
			result:      12345e6 + 6e5, // 12345.6 SKY
		},
		{
			maxDecimals: 1,
			satoshis:    876543219e4, // 87654.3219 BTC
			rate:        "2/3",
			result:      58436e6 + 2e5, // 58436.2 SKY
		},

		{
			maxDecimals: 2,
			satoshis:    1e8, // 1 BTC
			rate:        "1/2",
			result:      5e5, // 0.5 SKY
		},
		{
			maxDecimals: 2,
			satoshis:    12345e8, // 12345 BTC
			rate:        "1/2",
			result:      6172e6 + 5e5, // 6172.5 SKY
		},
		{
			maxDecimals: 2,
			satoshis:    1e8,
			rate:        "0.0001",
			result:      0, // 0 SKY
		},
		{
			maxDecimals: 2,
			satoshis:    12345678, // 0.12345678 BTC
			rate:        "512",
			result:      63e6 + 2e5, // 63.2 SKY
		},
		{
			maxDecimals: 2,
			satoshis:    123456789, // 1.23456789 BTC
			rate:        "10000",
			result:      12345e6 + 6e5 + 7e4, // 12345.67 SKY
		},
		{
			maxDecimals: 2,
			satoshis:    876543219e4, // 87654.3219 BTC
			rate:        "2/3",
			result:      58436e6 + 2e5 + 1e4, // 58436.21 SKY
		},

		{
			maxDecimals: 3,
			satoshis:    1e8, // 1 BTC
			rate:        "1/2",
			result:      5e5, // 0.5 SKY
		},
		{
			maxDecimals: 3,
			satoshis:    12345e8, // 12345 BTC
			rate:        "1/2",
			result:      6172e6 + 5e5, // 6172.5 SKY
		},
		{
			maxDecimals: 3,
			satoshis:    1e8,
			rate:        "0.0001",
			result:      0, // 0 SKY
		},
		{
			maxDecimals: 3,
			satoshis:    12345678, // 0.12345678 BTC
			rate:        "512",
			result:      63e6 + 2e5 + 9e3, // 63.209 SKY
		},
		{
			maxDecimals: 3,
			satoshis:    123456789, // 1.23456789 BTC
			rate:        "10000",
			result:      12345e6 + 6e5 + 7e4 + 8e3, // 12345.678 SKY
		},
		{
			maxDecimals: 3,
			satoshis:    876543219e4, // 87654.3219 BTC
			rate:        "2/3",
			result:      58436e6 + 2e5 + 1e4 + 4e3, // 58436.214 SKY
		},

		{
			maxDecimals: 4,
			satoshis:    1e8,
			rate:        "0.0001",
			result:      1e2, // 0.0001 SKY
		},

		{
			maxDecimals: 3,
			satoshis:    125e4,
			rate:        "1250",
			result:      15e6 + 6e5 + 2e4 + 5e3, // 15.625 SKY
		},
	}

	for _, tc := range cases {
		name := fmt.Sprintf("satoshis=%d rate=%s maxDecimals=%d", tc.satoshis, tc.rate, tc.maxDecimals)
		t.Run(name, func(t *testing.T) {
			result, err := CalculateBtcSkyValue(tc.satoshis, tc.rate, tc.maxDecimals)
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
		maxDecimals int
		wei         *big.Int
		rate        string
		result      uint64
		err         error
	}{
		{
			maxDecimals: 0,
			wei:         big.NewInt(-1),
			rate:        "1",
			err:         errors.New("wei must be greater than or equal to 0"),
		},

		{
			maxDecimals: 0,
			wei:         big.NewInt(1),
			rate:        "-1",
			err:         errors.New("rate must be greater than zero"),
		},

		{
			maxDecimals: 0,
			wei:         big.NewInt(1),
			rate:        "0",
			err:         errors.New("rate must be greater than zero"),
		},

		{
			maxDecimals: 0,
			wei:         big.NewInt(1),
			rate:        "invalidrate",
			err:         errors.New("can't convert invalidrate to decimal: exponent is not numeric"),
		},
		{
			maxDecimals: 0,
			wei:         big.NewInt(1),
			rate:        "100k",
			err:         errors.New("can't convert 100k to decimal"),
		},
		{
			maxDecimals: 0,
			wei:         big.NewInt(1),
			rate:        "0.1b",
			err:         errors.New("can't convert 0.1b to decimal"),
		},
		{
			maxDecimals: 0,
			wei:         big.NewInt(1),
			rate:        "",
			err:         errors.New("can't convert  to decimal"),
		},

		{
			maxDecimals: 0,
			wei:         big.NewInt(0),
			rate:        "1",
			result:      0,
		},
		{
			maxDecimals: 0,
			wei:         big.NewInt(1000),
			rate:        "0.001",
			result:      0,
		},

		{
			maxDecimals: 0,
			wei:         big.NewInt(1e18),
			rate:        "1",
			result:      1e6,
		},

		{
			maxDecimals: 0,
			wei:         big.NewInt(1e18),
			rate:        "500",
			result:      500e6,
		},

		{
			maxDecimals: 0,
			wei:         big.NewInt(1).Mul(big.NewInt(100), big.NewInt(1e18)),
			rate:        "500",
			result:      50000e6,
		},

		{
			maxDecimals: 0,
			wei:         big.NewInt(2e15), // 0.002 ETH
			rate:        "500",            // 500 SKY/ETH = 1 SKY / 0.002 ETH
			result:      1e6,              // 1 SKY
		},

		{
			maxDecimals: 0,
			wei:         big.NewInt(1e18), // 1 ETH
			rate:        "1/2",
			result:      0, // 0.5 SKY
		},

		{
			maxDecimals: 0,
			wei:         big.NewInt(11345e13), // 0.11345 ETH
			rate:        "100",
			result:      11e6, // 11 SKY
		},
		{
			maxDecimals: 0,
			wei:         big.NewInt(1).Mul(big.NewInt(2245236), big.NewInt(1e14)), // 224.5236 ETH
			rate:        "200",
			result:      44904e6, // 44904 SKY
		},
		{
			maxDecimals: 0,
			wei:         big.NewInt(1).Mul(big.NewInt(2245236), big.NewInt(1e14)), // 224.5236 ETH
			rate:        "1568",
			result:      352053e6, // 352053(224.5236 * 1568=352053.0048) SKY
		},
		{
			maxDecimals: 0,
			wei:         big.NewInt(1).Mul(big.NewInt(2245236), big.NewInt(1e14)), // 224.5236 ETH
			rate:        "0.15",
			result:      33e6, // 33 SKY
		},
		{
			maxDecimals: 0,
			wei:         big.NewInt(1).Mul(big.NewInt(2245236), big.NewInt(1e14)), // 224.5236 ETH
			rate:        "2/3",
			result:      149e6, // 149 SKY
		},
		{
			maxDecimals: 1,
			wei:         big.NewInt(1).Mul(big.NewInt(2245236), big.NewInt(1e14)), // 224.5236 ETH
			rate:        "200",
			result:      44904e6 + 7e5, // 44904.7 SKY
		},
		{
			maxDecimals: 1,
			wei:         big.NewInt(1).Mul(big.NewInt(2245236), big.NewInt(1e14)), // 224.5236 ETH
			rate:        "1568",
			result:      352053e6, // 352053(224.5236 * 1568=352053.0048) SKY
		},
		{
			maxDecimals: 1,
			wei:         big.NewInt(1).Mul(big.NewInt(2245236), big.NewInt(1e14)), // 224.5236 ETH
			rate:        "0.15",
			result:      33e6 + 6e5, // 33.6 SKY
		},
		{
			maxDecimals: 1,
			wei:         big.NewInt(1).Mul(big.NewInt(2245236), big.NewInt(1e14)), // 224.5236 ETH
			rate:        "2/3",
			result:      149e6 + 6e5, // 149.6 SKY
		},
		{
			maxDecimals: 2,
			wei:         big.NewInt(1).Mul(big.NewInt(2245236), big.NewInt(1e14)), // 224.5236 ETH
			rate:        "200",
			result:      44904e6 + 7e5 + 2e4, // 44904.72 SKY
		},
		{
			maxDecimals: 2,
			wei:         big.NewInt(1).Mul(big.NewInt(2245236), big.NewInt(1e14)), // 224.5236 ETH
			rate:        "1568",
			result:      352053e6, // 352053.00(224.5236 * 1568=352053.0048) SKY
		},
		{
			maxDecimals: 2,
			wei:         big.NewInt(1).Mul(big.NewInt(2245236), big.NewInt(1e14)), // 224.5236 ETH
			rate:        "0.15",
			result:      33e6 + 6e5 + 7e4, // 33.67 SKY
		},
		{
			maxDecimals: 2,
			wei:         big.NewInt(1).Mul(big.NewInt(2245236), big.NewInt(1e14)), // 224.5236 ETH
			rate:        "2/3",
			result:      149e6 + 6e5 + 8e4, // 149 SKY
		},
		{
			maxDecimals: 3,
			wei:         big.NewInt(1).Mul(big.NewInt(2245236), big.NewInt(1e14)), // 224.5236 ETH
			rate:        "1568",
			result:      352053e6 + 4e3, // 352053.004(224.5236 * 1568=352053.0048) SKY
		},
		{
			maxDecimals: 3,
			wei:         big.NewInt(1).Mul(big.NewInt(2245236), big.NewInt(1e14)), // 224.5236 ETH
			rate:        "0.15",
			result:      33e6 + 6e5 + 7e4 + 8e3, // 33.678 SKY
		},
		{
			maxDecimals: 3,
			wei:         big.NewInt(1).Mul(big.NewInt(2245236), big.NewInt(1e14)), // 224.5236 ETH
			rate:        "2/3",
			result:      149e6 + 6e5 + 8e4 + 2e3, // 149.682 SKY
		},
	}

	for _, tc := range cases {
		name := fmt.Sprintf("wei=%d rate=%s maxDecimals=%d", tc.wei, tc.rate, tc.maxDecimals)
		t.Run(name, func(t *testing.T) {
			result, err := CalculateEthSkyValue(tc.wei, tc.rate, tc.maxDecimals)
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
