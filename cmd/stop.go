package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pminnebach/fcvm/vm"
)

var stopCmd = &cobra.Command{
	Use:   "stop <id>",
	Short: "Stop a running microVM",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := loadConfig()
		if err != nil {
			return err
		}
		if err := vm.NewManager(c).Stop(args[0]); err != nil {
			return err
		}
		fmt.Printf("stopped VM %q\n", args[0])
		return nil
	},
}

func init() {
	rootCmd.AddCommand(stopCmd)
}
