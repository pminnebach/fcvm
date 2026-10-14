package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pminnebach/fcvm/vm"
)

var restartCmd = &cobra.Command{
	Use:   "restart <id>",
	Short: "Stop a microVM if it is running, then start it again from its disk",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := loadConfig()
		if err != nil {
			return err
		}
		state, err := vm.NewManager(c).Restart(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "restarted VM %q (guest %s, pid %d)\n", state.ID, state.GuestIP, state.PID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(restartCmd)
}
