package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pminnebach/fcvm/guest"
	"github.com/pminnebach/fcvm/vm"
)

var execCmd = &cobra.Command{
	Use:   "exec <id> -- command...",
	Short: "Run a command inside a microVM via SSH",
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
		state, err := vm.LoadState(c.StateDir, id)
		if err != nil {
			return err
		}
		key := state.SSHKey
		if key == "" {
			key = c.SSHKey
		}
		return guest.Exec(state.GuestIP, key, cmdArgs)
	},
}

var shellCmd = &cobra.Command{
	Use:   "shell <id>",
	Short: "Open an interactive SSH shell in a microVM",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := loadConfig()
		if err != nil {
			return err
		}
		state, err := vm.LoadState(c.StateDir, args[0])
		if err != nil {
			return err
		}
		key := state.SSHKey
		if key == "" {
			key = c.SSHKey
		}
		return guest.Shell(state.GuestIP, key)
	},
}

var attachCmd = &cobra.Command{
	Use:   "attach <id>",
	Short: "Tail the microVM serial log (debug)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := loadConfig()
		if err != nil {
			return err
		}
		state, err := vm.LoadState(c.StateDir, args[0])
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "attaching to log %s (Ctrl+C to exit)\n", state.LogPath)
		return guest.TailFollow(state.LogPath)
	},
}

func init() {
	rootCmd.AddCommand(execCmd, shellCmd, attachCmd)
}
