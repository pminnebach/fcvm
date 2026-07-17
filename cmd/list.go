package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/pminnebach/fcvm/vm"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List microVMs",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := loadConfig()
		if err != nil {
			return err
		}
		states, err := vm.NewManager(c).List()
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tGUEST IP\tPID\tUPTIME")
		for _, s := range states {
			fmt.Fprintf(w, "%s\t%s\t%d\t%s\n", s.ID, s.GuestIP, s.PID, time.Since(s.StartedAt).Round(time.Second))
		}
		w.Flush()
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
