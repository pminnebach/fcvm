package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/pminnebach/fcvm/guest"
	"github.com/pminnebach/fcvm/rootfs"
)

var buildRootfsCmd = &cobra.Command{
	Use:   "build-rootfs",
	Short: "Build a rootfs ext4 image from a Dockerfile",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := loadConfig()
		if err != nil {
			return err
		}
		dockerfile, _ := cmd.Flags().GetString("dockerfile")
		if dockerfile == "" {
			return fmt.Errorf("--dockerfile is required")
		}
		tag, _ := cmd.Flags().GetString("tag")
		if tag == "" {
			tag = "fcvm-rootfs:latest"
		}
		output, _ := cmd.Flags().GetString("output")
		if output == "" {
			output = c.Rootfs
		}
		size, _ := cmd.Flags().GetString("size")
		key, err := guest.LoadOrCreateKey(c.SSHKey)
		if err != nil {
			return err
		}
		if err := rootfs.BuildFromDockerfile(dockerfile, tag, output, size, key.PublicKey); err != nil {
			return err
		}
		fmt.Printf("rootfs image written to %s\n", output)
		return nil
	},
}

func init() {
	buildRootfsCmd.Flags().String("dockerfile", "", "path to Dockerfile")
	buildRootfsCmd.Flags().String("tag", "fcvm-rootfs:latest", "docker image tag")
	buildRootfsCmd.Flags().String("output", "", "output ext4 path (default: config rootfs)")
	buildRootfsCmd.Flags().String("size", "4G", "rootfs image size (truncate format, e.g. 4G, 512M)")
	rootCmd.AddCommand(buildRootfsCmd)
}
