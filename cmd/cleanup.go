package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/fcvm/fcvm/vm"
)

var cleanupCmd = &cobra.Command{
	Use:   "cleanup [id]",
	Short: "Cleanup VM resources",
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
		fmt.Println("cleanup complete")
		return nil
	},
}

func init() {
	cleanupCmd.Flags().Bool("all", false, "cleanup all VMs")
	rootCmd.AddCommand(cleanupCmd)
}
