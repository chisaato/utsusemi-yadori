package adb

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"utsusemi/ctl/internal/core"
)

// Status 描述 ADB 当前的实时状态以及当前 IP 列表
type Status struct {
	AdbdRunning  bool     `json:"adbd_running"`  // init.svc.adbd == running
	UsbEnabled   bool     `json:"usb_enabled"`   // settings global adb_enabled == 1
	TcpipEnabled bool     `json:"tcpip_enabled"` // service.adb.tcp.port > 0
	TcpipPort    int      `json:"tcpip_port"`    // 当前监听端口，<=0 表示未开启
	IPs          []string `json:"ips"`           // 本机可用局域网 IPv4 列表 (wlan/eth等)
}

// Controller 负责与底层 Android adb/adbd 交互
type Controller interface {
	GetStatus(ctx context.Context) (Status, error)
	SetUsb(ctx context.Context, enable bool) error
	SetTcpip(ctx context.Context, enable bool, port int) error
	RestartAdbd(ctx context.Context) error
	Apply(ctx context.Context, s core.AdbSettings) error
}

type realController struct {
	runner CommandRunner
}

// CommandRunner 执行 shell 命令接口，便于单测 mock
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

type osRunner struct{}

func (osRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func New() Controller {
	return &realController{runner: osRunner{}}
}

func NewWithRunner(runner CommandRunner) Controller {
	return &realController{runner: runner}
}

func (c *realController) GetStatus(ctx context.Context) (Status, error) {
	var st Status

	// 1. adbd 运行状态 (getprop init.svc.adbd)
	if out, err := c.runner.Run(ctx, "getprop", "init.svc.adbd"); err == nil {
		st.AdbdRunning = (strings.TrimSpace(out) == "running")
	}

	// 2. USB 调试状态 (settings get global adb_enabled)
	if out, err := c.runner.Run(ctx, "settings", "get", "global", "adb_enabled"); err == nil {
		st.UsbEnabled = (strings.TrimSpace(out) == "1")
	}

	// 3. 网络调试端口 (getprop service.adb.tcp.port)
	if out, err := c.runner.Run(ctx, "getprop", "service.adb.tcp.port"); err == nil {
		p, err := strconv.Atoi(strings.TrimSpace(out))
		if err == nil && p > 0 {
			st.TcpipEnabled = true
			st.TcpipPort = p
		} else {
			st.TcpipEnabled = false
			st.TcpipPort = -1
		}
	} else {
		st.TcpipPort = -1
	}

	// 4. 获取本机 IP 列表
	st.IPs = getLocalIPv4s()

	return st, nil
}

func (c *realController) SetUsb(ctx context.Context, enable bool) error {
	val := "0"
	ctlCmd := "ctl.stop"
	if enable {
		val = "1"
		ctlCmd = "ctl.start"
	}

	// settings put global adb_enabled 1/0
	if _, err := c.runner.Run(ctx, "settings", "put", "global", "adb_enabled", val); err != nil {
		return fmt.Errorf("failed to set adb_enabled to %s: %w", val, err)
	}

	// 启动或停止 adbd
	if _, err := c.runner.Run(ctx, "setprop", ctlCmd, "adbd"); err != nil {
		// 备用方案：stop adbd / start adbd
		action := "stop"
		if enable {
			action = "start"
		}
		_, _ = c.runner.Run(ctx, action, "adbd")
	}

	return nil
}

func (c *realController) SetTcpip(ctx context.Context, enable bool, port int) error {
	portStr := "-1"
	if enable {
		if port <= 0 || port > 65535 {
			port = 5555
		}
		portStr = strconv.Itoa(port)
	}

	// 1. setprop service.adb.tcp.port <port>
	if _, err := c.runner.Run(ctx, "setprop", "service.adb.tcp.port", portStr); err != nil {
		return fmt.Errorf("failed to setprop service.adb.tcp.port: %w", err)
	}

	// 2. 重启 adbd 使端口生效
	return c.RestartAdbd(ctx)
}

func (c *realController) RestartAdbd(ctx context.Context) error {
	// 尝试 setprop ctl.restart adbd
	if _, err := c.runner.Run(ctx, "setprop", "ctl.restart", "adbd"); err == nil {
		return nil
	}

	// 备选方案: stop adbd 然后 start adbd
	_, _ = c.runner.Run(ctx, "stop", "adbd")
	time.Sleep(100 * time.Millisecond)
	_, err := c.runner.Run(ctx, "start", "adbd")
	return err
}

func (c *realController) Apply(ctx context.Context, s core.AdbSettings) error {
	// 设置 USB 调试
	if err := c.SetUsb(ctx, s.UsbEnabled); err != nil {
		return err
	}

	// 设置 TCP/IP 网络调试
	port := s.Port
	if port <= 0 {
		port = 5555
	}
	if err := c.SetTcpip(ctx, s.TcpipEnabled, port); err != nil {
		return err
	}

	return nil
}

// getLocalIPv4s 枚举本机非 loopback 的 IPv4 地址
func getLocalIPv4s() []string {
	var ips []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return ips
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			if ipv4 := ip.To4(); ipv4 != nil {
				ips = append(ips, ipv4.String())
			}
		}
	}
	return ips
}
