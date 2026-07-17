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

var downloadFirecrackerCmd = &cobra.Command{
	Use:   "firecracker",
	Short: "Download latest firecracker and jailer release binaries",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := loadConfig()
		if err != nil {
			return err
		}
		dest := filepath.Dir(c.FirecrackerBin)
		ver, err := assets.DownloadFirecracker(dest)
		if err != nil {
			return err
		}
		fmt.Printf("downloaded firecracker %s to %s\n", ver, dest)
		return nil
	},
}

var downloadJailerCmd = &cobra.Command{
	Use:   "jailer",
	Short: "Download jailer from release (or build with --build)",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := loadConfig()
		if err != nil {
			return err
		}
		dest := filepath.Dir(c.JailerBin)
		build, _ := cmd.Flags().GetBool("build")
		if build {
			if err := assets.DownloadJailerBuild(dest); err != nil {
				return err
			}
			fmt.Printf("built firecracker+jailer in %s\n", dest)
			return nil
		}
		ver, err := assets.DownloadFirecracker(dest)
		if err != nil {
			return err
		}
		fmt.Printf("downloaded jailer from release %s to %s\n", ver, dest)
		return nil
	},
}

var downloadKernelCmd = &cobra.Command{
	Use:   "kernel",
	Short: "Download kernel (Firecracker CI latest by default)",
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
			url, err = assets.LatestFirecrackerKernelURL()
			if err != nil {
				return fmt.Errorf("kernel URL: %w (pass --url or set kernel-url)", err)
			}
		}
		if err := assets.DownloadKernel(url, c.Kernel); err != nil {
			return err
		}
		fmt.Printf("kernel downloaded from %s to %s\n", url, c.Kernel)
		return nil
	},
}

var downloadRootfsCmd = &cobra.Command{
	Use:   "rootfs",
	Short: "Download rootfs from URL",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := loadConfig()
		if err != nil {
			return err
		}
		url, _ := cmd.Flags().GetString("url")
		if url == "" {
			return fmt.Errorf("--url is required")
		}
		if err := assets.DownloadRootfs(url, c.Rootfs); err != nil {
			return err
		}
		fmt.Printf("rootfs saved to %s\n", c.Rootfs)
		return nil
	},
}

func init() {
	downloadJailerCmd.Flags().Bool("build", false, "build jailer from source via devtool")
	downloadKernelCmd.Flags().String("url", "", "kernel download URL (default: kernel-url config or Firecracker CI latest)")
	downloadRootfsCmd.Flags().String("url", "", "rootfs download URL")
	downloadCmd.AddCommand(downloadFirecrackerCmd, downloadJailerCmd, downloadKernelCmd, downloadRootfsCmd)
	rootCmd.AddCommand(downloadCmd)
}
