package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/fcvm/fcvm/vm"
)

var startCmd = &cobra.Command{
	Use:   "start [id]",
	Short: "Start a Firecracker microVM (always via jailer)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := loadConfig()
		if err != nil {
			return err
		}
		id, _ := cmd.Flags().GetString("id")
		if id == "" && len(args) > 0 {
			id = args[0]
		}
		mounts, err := cmd.Flags().GetStringArray("mount")
		if err != nil {
			return err
		}
		for _, m := range mounts {
			mc, err := mountFlag(m)
			if err != nil {
				return err
			}
			c.Mounts = append(c.Mounts, mc)
		}
		envFile, _ := cmd.Flags().GetStringToString("env")
		for k, v := range envFile {
			c.Env[k] = v
		}

		mgr := vm.NewManager(c)
		state, err := mgr.Start(context.Background(), id)
		if err != nil {
			return err
		}
		fmt.Printf("started VM %q (guest %s, pid %d)\n", state.ID, state.GuestIP, state.PID)
		return nil
	},
}

func init() {
	startCmd.Flags().String("id", "", "VM identifier")
	startCmd.Flags().StringArray("mount", nil, "host:guest[:ro] mount (repeatable)")
	startCmd.Flags().StringToString("env", nil, "environment variables KEY=VALUE")
	rootCmd.AddCommand(startCmd)
}
