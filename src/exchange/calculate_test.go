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
		rate     int64
		result   uint64
		err      error
	}{
		{
			satoshis: -1,
			rate:     1,
			err:      errors.New("negative satoshis or negative skyPerBTC"),
		},

		{
			satoshis: 1,
			rate:     -1,
			err:      errors.New("negative satoshis or negative skyPerBTC"),
		},

		{
			satoshis: 1e8,
			rate:     1,
			result:   1e6,
		},

		{
			satoshis: 1e8,
			rate:     500,
			result:   500e6,
		},

		{
			satoshis: 100e8,
			rate:     500,
			result:   50000e6,
		},

		{
			satoshis: 2e5, // 0.002 BTC
			rate:     500, // 500 SKY/BTC = 1 SKY / 0.002 BTC
			result:   1e6, // 1 SKY
		},
	}

	for _, tc := range cases {
		name := fmt.Sprintf("satoshis=%d rate=%d", tc.satoshis, tc.rate)
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
