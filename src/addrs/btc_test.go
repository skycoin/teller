package addrs

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewBtcAddrsContainsInvalid(t *testing.T) {
	addresses := []string{
		"14JwrdSxYXPxSi6crLKVwR4k2dbjfVZ3xj",
		"1JNonvXRyZvZ4ZJ9PE8voyo67UQN1TpoGy",
		"1JrzSx8a9FVHHCkUFLB2CHULpbz4dTz5Ap",
		"bad",
	}

	expectedErr := errors.New("Invalid deposit address `bad`: Invalid address length")

	err := verifyBTCAddresses(addresses)
	require.Error(t, err)
	require.Equal(t, expectedErr, err)
}
