package vm

import (
	"fmt"
	"net"
	"path/filepath"

	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
	"github.com/firecracker-microvm/firecracker-go-sdk/client/models"

	"github.com/pminnebach/fcvm/config"
	"github.com/pminnebach/fcvm/network"
)

const (
	defaultKernelArgs = "console=ttyS0 reboot=k panic=1 net.ifnames=0 biosdevname=0"
	defaultLogLevel   = "Info"
	fcSocketName      = "firecracker.sock"
	fcLogName         = "firecracker.log"
)

// blockDrive is a data drive attached alongside the rootfs.
type blockDrive struct {
	Path     string
	ReadOnly bool
}

type machineBuildInput struct {
	ID          string
	RootfsPath  string
	BlockDrives []blockDrive
	TapDev      string
	TapIP       string
	GuestIP     string
	GuestMAC    string
	JailerUID   int
	JailerGID   int
}

func buildFirecrackerConfig(cfg config.Config, in machineBuildInput) firecracker.Config {
	kernelArgs := cfg.KernelArgs
	if kernelArgs == "" {
		kernelArgs = defaultKernelArgs
	}

	logLevel := cfg.LogLevel
	if logLevel == "" {
		logLevel = defaultLogLevel
	}

	uid, gid := in.JailerUID, in.JailerGID
	numa := cfg.Jailer.NumaNode

	drives := []models.Drive{{
		DriveID:      firecracker.String("rootfs"),
		PathOnHost:   firecracker.String(in.RootfsPath),
		IsRootDevice: firecracker.Bool(true),
		IsReadOnly:   firecracker.Bool(false),
	}}
	for j, d := range in.BlockDrives {
		drives = append(drives, models.Drive{
			DriveID:      firecracker.String(fmt.Sprintf("data%d", j)),
			PathOnHost:   firecracker.String(d.Path),
			IsRootDevice: firecracker.Bool(false),
			IsReadOnly:   firecracker.Bool(d.ReadOnly),
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
		VMID:              in.ID,
		SocketPath:        fcSocketName,
		LogPath:           fcLogName,
		LogLevel:          logLevel,
		KernelImagePath:   cfg.Kernel,
		KernelArgs:        kernelArgs,
		Drives:            drives,
		MachineCfg:        machineCfg,
		NetworkInterfaces: buildNetworkInterfaces(cfg, in),
		MmdsVersion:       firecracker.MMDSv2,
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
	return fcCfg
}

func buildNetworkInterfaces(cfg config.Config, in machineBuildInput) []firecracker.NetworkInterface {
	if cfg.Network.CNINetwork != "" {
		return []firecracker.NetworkInterface{{
			CNIConfiguration: &firecracker.CNIConfiguration{
				NetworkName: cfg.Network.CNINetwork,
				IfName:      network.DefaultCNIIfName,
				VMIfName:    "eth0",
			},
			AllowMMDS: true,
		}}
	}
	return []firecracker.NetworkInterface{{
		StaticConfiguration: &firecracker.StaticNetworkConfiguration{
			MacAddress:  in.GuestMAC,
			HostDevName: in.TapDev,
			IPConfiguration: &firecracker.IPConfiguration{
				IPAddr: net.IPNet{
					IP:   net.ParseIP(in.GuestIP),
					Mask: net.CIDRMask(30, 32),
				},
				Gateway:     net.ParseIP(in.TapIP),
				Nameservers: cfg.Network.Nameservers,
				IfName:      "eth0",
			},
		},
		AllowMMDS: true,
	}}
}

// jailerCreds returns the uid/gid for this VM index.
func jailerCreds(cfg config.Config, index int) (uid, gid int) {
	uid, gid = cfg.Jailer.UID, cfg.Jailer.GID
	if cfg.Jailer.PerVMUIDs {
		uid += index
		gid += index
	}
	return uid, gid
}
