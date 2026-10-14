package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pminnebach/fcvm/vm"
)

var startCmd = &cobra.Command{
	Use:   "start [id]",
	Short: "Start a Firecracker VM (always via jailer), or boot a stopped one again",
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
		// An existing VM keeps the shape it was created with, so flags that
		// would change it are an error rather than silently ignored.
		if _, err := vm.Inspect(c.StateDir, id); err == nil && id != "" {
			if len(mounts) > 0 || cmd.Flags().Changed("env") {
				return fmt.Errorf("VM %q already exists and keeps its saved configuration; --mount and --env only apply when a VM is created. Run `fcvm delete %s` and start it again to change them", id, id)
			}
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
		state, err := mgr.Start(cmd.Context(), id)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "started VM %q (guest %s, pid %d)\n", state.ID, state.GuestIP, state.PID)
		return nil
	},
}

func init() {
	startCmd.Flags().String("id", "", "VM identifier")
	startCmd.Flags().StringArray("mount", nil, "host:guest[:ro|rw|method=nfs|block|size=N] mount (repeatable)")
	startCmd.Flags().StringToString("env", nil, "environment variables KEY=VALUE")
	rootCmd.AddCommand(startCmd)
}
