package exchange

import (
	"errors"
	"fmt"
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
			result:   5e5, // 0.5 SKY
		},
	}

	for _, tc := range cases {
		name := fmt.Sprintf("satoshis=%d rate=%s", tc.satoshis, tc.rate)
		t.Run(name, func(t *testing.T) {
			result, err := calculateSkyValue(tc.satoshis, tc.rate)
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
