package cmd

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/pminnebach/fcvm/vm"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List microVMs",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := loadConfig()
		if err != nil {
			return err
		}
		states, err := vm.NewManager(c).List()
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tSTATUS\tGUEST IP\tPID\tUPTIME")
		for _, s := range states {
			// A crashed VM keeps its state file, so status comes from the
			// process, not from the file existing.
			status, uptime := "stopped", "-"
			if s.IsRunning() {
				status = "running"
				uptime = time.Since(s.StartedAt).Round(time.Second).String()
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n", s.ID, status, s.GuestIP, s.PID, uptime)
		}
		return w.Flush()
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
