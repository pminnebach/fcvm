package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/pminnebach/fcvm/guest"
	"github.com/pminnebach/fcvm/vm"
	"github.com/pminnebach/fcvm/vsock"
)

var vsockExecCmd = &cobra.Command{
	Use:   "vsock-exec <id> -- command...",
	Short: "Run a command inside a microVM via vsock",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := loadConfig()
		if err != nil {
			return err
		}
		id := args[0]
		cmdArgs := args[1:]
		if cmdArgs[0] == "--" {
			cmdArgs = cmdArgs[1:]
		}
		if len(cmdArgs) == 0 {
			return fmt.Errorf("missing command")
		}
		state, err := vm.LoadState(c.StateDir, id)
		if err != nil {
			return err
		}
		if state.VsockUDS == "" {
			return fmt.Errorf("VM %q has no vsock device (restart with --enable-vsock)", id)
		}
		command := guest.RemoteCommand(cmdArgs)
		timeout := time.Duration(c.WaitTimeoutSec) * time.Second
		ctx := cmd.Context()
		if err := guest.WaitVsock(ctx, state.VsockUDS, timeout); err != nil {
			return err
		}
		err = guest.VsockExec(ctx, state.VsockUDS, command, cmd.OutOrStdout())
		if err == nil {
			return nil
		}
		if e, ok := err.(*vsock.ExitError); ok {
			os.Exit(e.Code)
		}
		return err
	},
}

func init() {
	rootCmd.AddCommand(vsockExecCmd)
}
