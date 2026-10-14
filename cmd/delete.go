package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pminnebach/fcvm/vm"
)

var deleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a VM, its disk and its reserved network slot",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := loadConfig()
		if err != nil {
			return err
		}
		force, _ := cmd.Flags().GetBool("force")
		if err := vm.NewManager(c).Delete(args[0], force); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "deleted VM %q\n", args[0])
		return nil
	},
}

func init() {
	deleteCmd.Flags().Bool("force", false, "stop the VM first if it is running")
	rootCmd.AddCommand(deleteCmd)
}
