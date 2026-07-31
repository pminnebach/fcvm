package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"

	"github.com/pminnebach/fcvm/config"
)

// experimentalIO is overridable in tests.
var (
	experimentalStdin  io.Reader = os.Stdin
	experimentalStderr io.Writer = os.Stderr
)

type experimentalItem struct {
	Feature string
	Kind    string // command or flag
	Name    string
	Note    string
}

func experimentalItems() []experimentalItem {
	return []experimentalItem{
		{Feature: "vsock", Kind: "command", Name: "vsock-exec", Note: "run a command in the guest over vsock"},
		{Feature: "vsock", Kind: "flag", Name: "--enable-vsock", Note: "attach virtio-vsock and inject the guest agent"},
		{Feature: "vsock", Kind: "flag", Name: "--guest-agent-bin", Note: "guest agent binary path (used with --enable-vsock)"},
		{Feature: "cni", Kind: "flag", Name: "--cni-network", Note: "CNI networking instead of static TAP"},
	}
}

func experimentalReasons(cmd *cobra.Command, c config.Config) []string {
	var reasons []string
	if cmd != nil && cmd.Name() == "vsock-exec" {
		reasons = append(reasons, "command: vsock-exec")
	}
	if c.EnableVsock {
		reasons = append(reasons, "flag: --enable-vsock")
	}
	if c.Network.CNINetwork != "" {
		reasons = append(reasons, "flag: --cni-network")
	}
	if cmd != nil {
		if f := cmd.Flags().Lookup("guest-agent-bin"); f != nil && f.Changed {
			reasons = append(reasons, "flag: --guest-agent-bin")
		} else if f := cmd.InheritedFlags().Lookup("guest-agent-bin"); f != nil && f.Changed {
			reasons = append(reasons, "flag: --guest-agent-bin")
		} else if root := cmd.Root(); root != nil {
			if f := root.PersistentFlags().Lookup("guest-agent-bin"); f != nil && f.Changed {
				reasons = append(reasons, "flag: --guest-agent-bin")
			}
		}
	}
	return reasons
}

func confirmExperimental(cmd *cobra.Command, c config.Config, reasons []string) error {
	if c.EnableExperimental || len(reasons) == 0 {
		return nil
	}
	out := experimentalStderr
	if cmd != nil {
		out = cmd.ErrOrStderr()
	}
	fmt.Fprintln(out, "WARNING: experimental features in use:")
	for _, r := range reasons {
		fmt.Fprintf(out, "  - %s\n", r)
	}
	fmt.Fprintln(out, "See: fcvm experimental")
	fmt.Fprintln(out, "Pass --enable-experimental to skip this prompt.")

	in := experimentalStdin
	if f, ok := in.(*os.File); ok && !isTerminal(f) {
		return fmt.Errorf("experimental features require confirmation; re-run with --enable-experimental")
	}
	fmt.Fprint(out, "Continue? [y/N]: ")
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && err != io.EOF {
		return fmt.Errorf("experimental confirmation: %w", err)
	}
	ans := strings.TrimSpace(strings.ToLower(line))
	if ans == "y" || ans == "yes" {
		return nil
	}
	return fmt.Errorf("aborted: experimental features not confirmed")
}

func isTerminal(f *os.File) bool {
	_, err := unix.IoctlGetTermios(int(f.Fd()), unix.TCGETS)
	return err == nil
}

var experimentalCmd = &cobra.Command{
	Use:   "experimental",
	Short: "List experimental commands and flags",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "FEATURE\tKIND\tNAME\tNOTE")
		for _, item := range experimentalItems() {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", item.Feature, item.Kind, item.Name, item.Note)
		}
		return w.Flush()
	},
}

func init() {
	rootCmd.AddCommand(experimentalCmd)
}
