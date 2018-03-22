package addrs

import (
	"bytes"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/MDLlife/teller/src/util/testutil"
)

func TestNewWAVESAddrsAllValid(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	addressesJSON := `
3PJaDyprvekvPXPuAtxrapacuDJopgJRaU3
3PFTGLDvE7rQfWtgSzBt7NS4NXXMQ1gUufs
3P9dUze9nHRdfoKhFrZYKdsSpwW9JoE6Mzf
`

	wavesAddrMgr, err := NewWAVESAddrs(log, db, bytes.NewReader([]byte(addressesJSON)))

	require.Nil(t, err)
	require.NotNil(t, wavesAddrMgr)
}

func TestNewWAVESAddrsContainsInvalid(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	addressesJSON := `
3P28Lsv1Pxf63EnvqoymXwbhQ1GnFFH5s6C
bad
`

	expectedErr := errors.New("Invalid deposit address `bad`: Invalid address length")

	wavesAddrMgr, err := NewWAVESAddrs(log, db, bytes.NewReader([]byte(addressesJSON)))

	require.Error(t, err)
	require.Equal(t, expectedErr, err)
	require.Nil(t, wavesAddrMgr)
}

func TestNewWAVESAddrsContainsDuplicated(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	addressesJSON := `
3PEruAtC1edYhUPNoNAerP5xjdVaQMDHkPP
3P3Y5U5CbJNHfszLa9JUieR91Et14yuLsRs
3P5DKriHBPkeUN1GJXsq5S2tPwXqxw2f1Nr
3P5DKriHBPkeUN1GJXsq5S2tPwXqxw2f1Nr
`

	expectedErr := errors.New("Duplicate deposit address `3P5DKriHBPkeUN1GJXsq5S2tPwXqxw2f1Nr`")

	wavesAddrMgr, err := NewWAVESAddrs(log, db, bytes.NewReader([]byte(addressesJSON)))

	require.Error(t, err)
	require.Equal(t, expectedErr, err)
	require.Nil(t, wavesAddrMgr)
}

func TestNewWAVESAddrsContainsNull(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	addressesJSON := ``

	expectedErr := errors.New("No WAVES addresses")

	wavesAddrMgr, err := NewWAVESAddrs(log, db, bytes.NewReader([]byte(addressesJSON)))

	require.Error(t, err)
	require.Equal(t, expectedErr, err)
	require.Nil(t, wavesAddrMgr)
}
