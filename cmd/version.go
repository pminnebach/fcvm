package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Set at build time via -ldflags (see .goreleaser.yaml).
var version = "dev"

// supportedFirecrackerVersion is the Firecracker API/SDK floor this tree targets.
const supportedFirecrackerVersion = "1.0.0"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print fcvm version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("fcvm %s (supported Firecracker %s)\n", version, supportedFirecrackerVersion)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
