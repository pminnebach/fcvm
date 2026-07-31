package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pminnebach/fcvm/vm"
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup [id]",
	Short: "Cleanup VM resources",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := loadConfig()
		if err != nil {
			return err
		}
		all, _ := cmd.Flags().GetBool("all")
		id := ""
		if len(args) > 0 {
			id = args[0]
		}
		if err := vm.NewManager(c).Cleanup(all, id); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "cleanup complete")
		return nil
	},
}

func init() {
	cleanupCmd.Flags().Bool("all", false, "cleanup all VMs")
	rootCmd.AddCommand(cleanupCmd)
}
