package cli

import (
	gcli "github.com/urfave/cli"

	"github.com/MDLlife/MDL/src/api/webrpc"
)

func statusCmd() gcli.Command {
	name := "status"
	return gcli.Command{
		Name:         name,
		Usage:        "Check the status of current mdl node",
		ArgsUsage:    " ",
		OnUsageError: onCommandUsageError(name),
		Action: func(c *gcli.Context) error {
			rpcClient := RpcClientFromContext(c)
			status, err := rpcClient.GetStatus()
			if err != nil {
				return err
			}

			return printJson(struct {
				webrpc.StatusResult
				RPCAddress string `json:"webrpc_address"`
			}{
				StatusResult: *status,
				RPCAddress:   rpcClient.Addr,
			})
		},
	}
}
