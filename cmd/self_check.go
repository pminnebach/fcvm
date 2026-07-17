package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/pminnebach/fcvm/vm"
)

var selfCheckCmd = &cobra.Command{
	Use:   "self-check",
	Short: "Run integration self-check (requires KVM and root)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if _, err := os.Stat("/dev/kvm"); err != nil {
			fmt.Println("skip: /dev/kvm not available")
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
		state, err := mgr.Start(context.Background(), id)
		if err != nil {
			return fmt.Errorf("start: %w", err)
		}
		if err := mgr.Stop(id); err != nil {
			return fmt.Errorf("stop: %w", err)
		}
		fmt.Printf("self-check ok (VM %s at %s)\n", state.ID, state.GuestIP)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(selfCheckCmd)
}
