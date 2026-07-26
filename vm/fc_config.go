package vm

import (
	"fmt"
	"net"
	"path/filepath"
	"strings"

	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"

	"github.com/pminnebach/fcvm/config"
)

const (
	defaultKernelArgs = "console=ttyS0 reboot=k panic=1 net.ifnames=0 biosdevname=0"
	defaultLogLevel   = "Info"
	fcSocketName      = "firecracker.sock"
	fcLogName         = "firecracker.log"
)

type machineBuildInput struct {
	ID          string
	RootfsPath  string
	BlockImages []string
	TapDev      string
	TapIP       string
	GuestIP     string
	GuestMAC    string
}

func buildFirecrackerConfig(cfg config.Config, in machineBuildInput) (firecracker.Config, error) {
	kernelArgs := cfg.KernelArgs
	if kernelArgs == "" {
		kernelArgs = defaultKernelArgs
	}
	kernelArgs = applyExposeKVMArgs(kernelArgs, cfg.ExposeKVM)

	logLevel := cfg.LogLevel
	if logLevel == "" {
		logLevel = defaultLogLevel
	}

	uid, gid := cfg.Jailer.UID, cfg.Jailer.GID
	numa := cfg.Jailer.NumaNode

	drives := []models.Drive{{
		DriveID:      firecracker.String("rootfs"),
		PathOnHost:   firecracker.String(in.RootfsPath),
		IsRootDevice: firecracker.Bool(true),
		IsReadOnly:   firecracker.Bool(false),
	}}
	for j, img := range in.BlockImages {
		drives = append(drives, models.Drive{
			DriveID:      firecracker.String(fmt.Sprintf("data%d", j)),
			PathOnHost:   firecracker.String(img),
			IsRootDevice: firecracker.Bool(false),
			IsReadOnly:   firecracker.Bool(false),
		})
	}

	machineCfg := models.MachineConfiguration{
		VcpuCount:  firecracker.Int64(cfg.VCPUCount),
		MemSizeMib: firecracker.Int64(cfg.MemSizeMib),
	}
	if cfg.CPUTemplate != "" {
		machineCfg.CPUTemplate = models.CPUTemplate(cfg.CPUTemplate)
	}
	if cfg.DisableSMT {
		machineCfg.Smt = firecracker.Bool(false)
	}

	fcCfg := firecracker.Config{
		VMID:            in.ID,
		SocketPath:      fcSocketName,
		LogPath:         fcLogName,
		LogLevel:        logLevel,
		KernelImagePath: cfg.Kernel,
		KernelArgs:      kernelArgs,
		Drives:          drives,
		MachineCfg:      machineCfg,
		NetworkInterfaces: []firecracker.NetworkInterface{{
			StaticConfiguration: &firecracker.StaticNetworkConfiguration{
				MacAddress:  in.GuestMAC,
				HostDevName: in.TapDev,
				IPConfiguration: &firecracker.IPConfiguration{
					IPAddr: net.IPNet{
						IP:   net.ParseIP(in.GuestIP),
						Mask: net.CIDRMask(30, 32),
					},
					Gateway:     net.ParseIP(in.TapIP),
					Nameservers: []string{"8.8.8.8"},
					IfName:      "eth0",
				},
			},
			AllowMMDS: true,
		}},
		MmdsVersion: firecracker.MMDSv2,
		JailerCfg: &firecracker.JailerConfig{
			JailerBinary:  cfg.JailerBin,
			ExecFile:      cfg.FirecrackerBin,
			ID:            in.ID,
			UID:           &uid,
			GID:           &gid,
			NumaNode:      &numa,
			Daemonize:     cfg.Jailer.Daemonize,
			CgroupArgs:    cfg.Jailer.Cgroup,
			ParentCgroup:  cfg.Jailer.ParentCgroup,
			CgroupVersion: jailerCgroupVersion(),
			ChrootBaseDir: cfg.Jailer.ChrootBaseDir,
			ChrootStrategy: firecracker.NewNaiveChrootStrategy(
				filepath.Base(cfg.Kernel),
			),
		},
	}
	return fcCfg, nil
}

func applyExposeKVMArgs(kernelArgs string, expose bool) string {
	if !expose {
		return kernelArgs
	}
	tokens := []string{"pci=on", "fcvm.kvm=1"}
	for _, t := range tokens {
		if !kernelArgsContainsToken(kernelArgs, t) {
			kernelArgs += " " + t
		}
	}
	return kernelArgs
}

func kernelArgsContainsToken(args, token string) bool {
	for _, f := range strings.Fields(args) {
		if f == token {
			return true
		}
	}
	return false
}
