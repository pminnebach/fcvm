package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Set at build time via -ldflags (see .goreleaser.yaml).
var version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print fcvm version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
