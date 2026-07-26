package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/pminnebach/fcvm/vm"
)

var selfCheckCmd = &cobra.Command{
	Use:   "self-check",
	Short: "Run integration self-check (requires KVM and root)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := os.Stat("/dev/kvm"); err != nil {
			fmt.Fprintln(cmd.OutOrStdout(), "skip: /dev/kvm not available")
			return nil
		}
		if os.Geteuid() != 0 {
			return fmt.Errorf("self-check requires root")
		}
		c, err := loadConfig()
		if err != nil {
			return err
		}
		mgr := vm.NewManager(c)
		id := "selfcheck"
		_ = mgr.Cleanup(false, id)
		state, err := mgr.Start(cmd.Context(), id)
		if err != nil {
			return fmt.Errorf("start: %w", err)
		}
		if err := mgr.Stop(id); err != nil {
			return fmt.Errorf("stop: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "self-check ok (VM %s at %s)\n", state.ID, state.GuestIP)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(selfCheckCmd)
}
