package cli

import (
	gcli "github.com/urfave/cli"

	"github.com/MDLlife/MDL/src/cipher"
)

func verifyAddressCmd() gcli.Command {
	name := "verifyAddress"
	return gcli.Command{
		Name:         name,
		Usage:        "Verify a MDL address",
		ArgsUsage:    "[mdl address]",
		OnUsageError: onCommandUsageError(name),
		Action: func(c *gcli.Context) error {
			mdlAddr := c.Args().First()
			_, err := cipher.DecodeBase58Address(mdlAddr)
			return err
		},
	}
}
