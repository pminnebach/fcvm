package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/pminnebach/fcvm/assets"
)

var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download Firecracker assets",
}

// fetchOptions reads the verification flags shared by the download commands.
func fetchOptions(cmd *cobra.Command) assets.FetchOptions {
	sum, _ := cmd.Flags().GetString("sha256")
	insecure, _ := cmd.Flags().GetBool("insecure")
	return assets.FetchOptions{SHA256: sum, Insecure: insecure}
}

var downloadFirecrackerCmd = &cobra.Command{
	Use:   "firecracker",
	Short: "Download latest firecracker and jailer release binaries",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := loadConfig()
		if err != nil {
			return err
		}
		dest := filepath.Dir(c.FirecrackerBin)
		ver, err := assets.DownloadFirecracker(cmd.Context(), dest)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "downloaded firecracker %s to %s (checksum verified)\n", ver, dest)
		return nil
	},
}

var downloadJailerCmd = &cobra.Command{
	Use:   "jailer",
	Short: "Download jailer from release (or build with --build)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := loadConfig()
		if err != nil {
			return err
		}
		dest := filepath.Dir(c.JailerBin)
		build, _ := cmd.Flags().GetBool("build")
		if build {
			if err := assets.DownloadJailerBuild(cmd.Context(), dest); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "built firecracker+jailer in %s\n", dest)
			return nil
		}
		ver, err := assets.DownloadFirecracker(cmd.Context(), dest)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "downloaded jailer from release %s to %s\n", ver, dest)
		return nil
	},
}

var downloadKernelCmd = &cobra.Command{
	Use:   "kernel",
	Short: "Download kernel (Firecracker CI latest by default)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := loadConfig()
		if err != nil {
			return err
		}
		url, _ := cmd.Flags().GetString("url")
		if url == "" {
			url = c.KernelURL
		}
		if url == "" {
			url, err = assets.LatestFirecrackerKernelURL(cmd.Context())
			if err != nil {
				return fmt.Errorf("kernel URL: %w (pass --url or set kernel-url)", err)
			}
		}
		if err := assets.DownloadKernel(cmd.Context(), url, c.Kernel, fetchOptions(cmd)); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "kernel downloaded from %s to %s\n", url, c.Kernel)
		return nil
	},
}

var downloadRootfsCmd = &cobra.Command{
	Use:   "rootfs",
	Short: "Download rootfs from URL",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := loadConfig()
		if err != nil {
			return err
		}
		url, _ := cmd.Flags().GetString("url")
		size, _ := cmd.Flags().GetString("size")
		if err := assets.DownloadRootfs(cmd.Context(), url, c.Rootfs, size, fetchOptions(cmd)); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "rootfs saved to %s\n", c.Rootfs)
		return nil
	},
}

var downloadGuestAgentCmd = &cobra.Command{
	Use:   "guest-agent",
	Short: "Download fcvm-guest-agent (experimental)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := loadConfig()
		if err != nil {
			return err
		}
		url, _ := cmd.Flags().GetString("url")
		if url != "" {
			if err := assets.DownloadGuestAgentURL(cmd.Context(), url, c.GuestAgentBin, fetchOptions(cmd)); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "guest-agent downloaded from %s to %s\n", url, c.GuestAgentBin)
			return nil
		}
		if err := assets.DownloadGuestAgent(cmd.Context(), version, c.GuestAgentBin); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "downloaded guest-agent %s to %s (checksum verified)\n", version, c.GuestAgentBin)
		return nil
	},
}

func init() {
	downloadJailerCmd.Flags().Bool("build", false, "build jailer from source via devtool")
	downloadKernelCmd.Flags().String("url", "", "kernel download URL (default: kernel-url config or Firecracker CI latest)")
	downloadRootfsCmd.Flags().String("url", "", "rootfs download URL")
	downloadRootfsCmd.Flags().String("size", "", "ext4 size when converting a squashfs (default: sized from contents)")
	_ = downloadRootfsCmd.MarkFlagRequired("url")
	downloadGuestAgentCmd.Flags().String("url", "", "guest-agent binary URL (default: GitHub release matching this fcvm version)")
	for _, c := range []*cobra.Command{downloadKernelCmd, downloadRootfsCmd, downloadGuestAgentCmd} {
		c.Flags().String("sha256", "", "expected SHA-256 of the download")
		c.Flags().Bool("insecure", false, "allow a plain http:// URL")
	}
	downloadCmd.AddCommand(downloadFirecrackerCmd, downloadJailerCmd, downloadKernelCmd, downloadRootfsCmd, downloadGuestAgentCmd)
	rootCmd.AddCommand(downloadCmd)
}
