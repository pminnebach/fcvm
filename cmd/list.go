package cmd

import (
	"fmt"
	"io"
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
		infos, err := vm.NewManager(c).List()
		if err != nil {
			return err
		}
		return renderList(cmd.OutOrStdout(), infos, time.Now())
	},
}

// renderList prints one row per VM. Status comes from comparing the records on
// disk with the real process (see vm.Inspect), so a VM the user stopped reads
// differently from one that died.
func renderList(out io.Writer, infos []vm.Info, now time.Time) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSTATUS\tGUEST IP\tPID\tUPTIME\tSTOPPED")
	for _, info := range infos {
		pid, uptime, stopped := "-", "-", "-"
		if info.Status == vm.StatusRunning {
			pid = fmt.Sprint(info.State.PID)
			uptime = now.Sub(info.State.StartedAt).Round(time.Second).String()
		}
		if info.Status == vm.StatusStopped && !info.StoppedAt.IsZero() {
			stopped = now.Sub(info.StoppedAt).Round(time.Second).String() + " ago"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", info.ID, info.Status, listGuestIP(info), pid, uptime, stopped)
	}
	return w.Flush()
}

// listGuestIP shows the address a VM has or will come back on. A CNI VM is
// handed a new address at each start, so a VM that is not running has none to
// promise.
func listGuestIP(info vm.Info) string {
	ip := ""
	switch {
	case info.Status == vm.StatusRunning || info.Status == vm.StatusLegacy:
		ip = info.State.GuestIP
	case info.Config != nil && !info.Config.IsCNI():
		ip = info.Config.GuestIP
	}
	if ip == "" {
		return "-"
	}
	return ip
}

func init() {
	rootCmd.AddCommand(listCmd)
}
