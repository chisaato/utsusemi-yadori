package cmd

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"utsusemi/ctl/internal/adb"
	"utsusemi/ctl/internal/core"
)

type adbStatusView struct {
	AdbdRunning  bool              `json:"adbd_running"`
	UsbEnabled   bool              `json:"usb_enabled"`
	TcpipEnabled bool              `json:"tcpip_enabled"`
	TcpipPort    int               `json:"tcpip_port"`
	IPs          []string          `json:"ips"`
	Settings     core.AdbSettings  `json:"settings"`
}

func (v adbStatusView) String() string {
	var b strings.Builder
	adbd := "stopped"
	if v.AdbdRunning {
		adbd = "running"
	}
	usb := "off"
	if v.UsbEnabled {
		usb = "on"
	}
	tcp := "off"
	if v.TcpipEnabled {
		tcp = fmt.Sprintf("port %d", v.TcpipPort)
	}

	b.WriteString(fmt.Sprintf("adbd daemon:   %s\n", adbd))
	b.WriteString(fmt.Sprintf("usb debugging: %s\n", usb))
	b.WriteString(fmt.Sprintf("tcpip debugging: %s\n", tcp))

	if len(v.IPs) > 0 {
		b.WriteString(fmt.Sprintf("device ips:    %s\n", strings.Join(v.IPs, ", ")))
		if v.TcpipEnabled && v.TcpipPort > 0 {
			b.WriteString("connect command:\n")
			for _, ip := range v.IPs {
				b.WriteString(fmt.Sprintf("  adb connect %s:%d\n", ip, v.TcpipPort))
			}
		}
	} else {
		b.WriteString("device ips:    (none found)\n")
	}

	b.WriteString("saved settings:\n")
	b.WriteString(fmt.Sprintf("  usb_enabled:   %v\n", v.Settings.UsbEnabled))
	b.WriteString(fmt.Sprintf("  tcpip_enabled: %v\n", v.Settings.TcpipEnabled))
	b.WriteString(fmt.Sprintf("  default_port:  %d\n", v.Settings.Port))
	b.WriteString(fmt.Sprintf("  apply_on_boot: %v", v.Settings.ApplyOnBoot))

	return b.String()
}

func newAdbCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "adb",
		Short: "ADB 管理与快速开关",
	}

	c.AddCommand(
		newAdbStatusCmd(),
		newAdbOnCmd(),
		newAdbOffCmd(),
		newAdbToggleCmd(),
		newAdbTcpipCmd(),
		newAdbRestartCmd(),
		newAdbSetCmd(),
		newAdbApplyCmd(),
	)

	return c
}

func newAdbStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "查看 ADB 实时状态与已保存设置",
		RunE: func(c *cobra.Command, _ []string) error {
			s, err := core.LoadSettings(paths())
			if err != nil {
				return fail(c, err)
			}
			ctrl := adb.New()
			st, err := ctrl.GetStatus(context.Background())
			if err != nil {
				return fail(c, err)
			}
			view := adbStatusView{
				AdbdRunning:  st.AdbdRunning,
				UsbEnabled:   st.UsbEnabled,
				TcpipEnabled: st.TcpipEnabled,
				TcpipPort:    st.TcpipPort,
				IPs:          st.IPs,
				Settings:     s.Adb,
			}
			emit(c, Envelope{OK: true, Data: view})
			return nil
		},
	}
}

func newAdbOnCmd() *cobra.Command {
	var save bool
	cmd := &cobra.Command{
		Use:   "on",
		Short: "开启 USB 调试",
		RunE: func(c *cobra.Command, _ []string) error {
			ctrl := adb.New()
			if err := ctrl.SetUsb(context.Background(), true); err != nil {
				return fail(c, err)
			}
			if save {
				s, err := core.LoadSettings(paths())
				if err == nil {
					s.Adb.UsbEnabled = true
					_ = core.SaveSettings(paths(), s)
				}
			}
			emit(c, Envelope{OK: true, Data: map[string]any{"usb_enabled": true, "saved": save}})
			return nil
		},
	}
	cmd.Flags().BoolVar(&save, "save", false, "同时保存到配置文件")
	return cmd
}

func newAdbOffCmd() *cobra.Command {
	var save bool
	cmd := &cobra.Command{
		Use:   "off",
		Short: "关闭 USB 调试",
		RunE: func(c *cobra.Command, _ []string) error {
			ctrl := adb.New()
			if err := ctrl.SetUsb(context.Background(), false); err != nil {
				return fail(c, err)
			}
			if save {
				s, err := core.LoadSettings(paths())
				if err == nil {
					s.Adb.UsbEnabled = false
					_ = core.SaveSettings(paths(), s)
				}
			}
			emit(c, Envelope{OK: true, Data: map[string]any{"usb_enabled": false, "saved": save}})
			return nil
		},
	}
	cmd.Flags().BoolVar(&save, "save", false, "同时保存到配置文件")
	return cmd
}

func newAdbToggleCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "toggle",
		Short: "切换 USB 调试开启/关闭状态",
		RunE: func(c *cobra.Command, _ []string) error {
			ctrl := adb.New()
			st, err := ctrl.GetStatus(context.Background())
			if err != nil {
				return fail(c, err)
			}
			target := !st.UsbEnabled
			if err := ctrl.SetUsb(context.Background(), target); err != nil {
				return fail(c, err)
			}
			emit(c, Envelope{OK: true, Data: map[string]any{"usb_enabled": target}})
			return nil
		},
	}
}

func newAdbTcpipCmd() *cobra.Command {
	var save bool
	cmd := &cobra.Command{
		Use:   "tcpip [port|off]",
		Short: "开启/关闭无线网络调试（默认端口 5555，输入 off 关闭）",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			enable := true
			port := 5555

			if len(args) > 0 {
				arg := strings.ToLower(strings.TrimSpace(args[0]))
				if arg == "off" || arg == "disable" || arg == "-1" {
					enable = false
					port = -1
				} else {
					p, err := strconv.Atoi(arg)
					if err != nil || p <= 0 || p > 65535 {
						return fail(c, fmt.Errorf("invalid port %q: must be 1-65535", args[0]))
					}
					port = p
				}
			}

			ctrl := adb.New()
			if err := ctrl.SetTcpip(context.Background(), enable, port); err != nil {
				return fail(c, err)
			}

			if save {
				s, err := core.LoadSettings(paths())
				if err == nil {
					s.Adb.TcpipEnabled = enable
					if enable {
						s.Adb.Port = port
					}
					_ = core.SaveSettings(paths(), s)
				}
			}

			st, _ := ctrl.GetStatus(context.Background())
			emit(c, Envelope{OK: true, Data: map[string]any{
				"tcpip_enabled": enable,
				"tcpip_port":    port,
				"saved":         save,
				"ips":           st.IPs,
			}})
			return nil
		},
	}
	cmd.Flags().BoolVar(&save, "save", false, "同时保存到配置文件")
	return cmd
}

func newAdbRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "重启 adbd 服务",
		RunE: func(c *cobra.Command, _ []string) error {
			ctrl := adb.New()
			if err := ctrl.RestartAdbd(context.Background()); err != nil {
				return fail(c, err)
			}
			emit(c, Envelope{OK: true, Data: map[string]bool{"restarted": true}})
			return nil
		},
	}
}

func newAdbSetCmd() *cobra.Command {
	var (
		usb   *bool
		tcp   *bool
		port  int
		boot  *bool
		apply bool
	)
	cmd := &cobra.Command{
		Use:   "set",
		Short: "修改并保存 ADB 配置，可选 --apply 立即应用生效",
		RunE: func(c *cobra.Command, _ []string) error {
			s, err := core.LoadSettings(paths())
			if err != nil {
				return fail(c, err)
			}
			if c.Flags().Changed("usb") {
				s.Adb.UsbEnabled = *usb
			}
			if c.Flags().Changed("tcpip") {
				s.Adb.TcpipEnabled = *tcp
			}
			if c.Flags().Changed("port") {
				if port <= 0 || port > 65535 {
					return fail(c, fmt.Errorf("invalid port %d: must be 1-65535", port))
				}
				s.Adb.Port = port
			}
			if c.Flags().Changed("boot") {
				s.Adb.ApplyOnBoot = *boot
			}

			if err := core.SaveSettings(paths(), s); err != nil {
				return fail(c, err)
			}

			applied := false
			if apply {
				ctrl := adb.New()
				if err := ctrl.Apply(context.Background(), s.Adb); err != nil {
					return fail(c, fmt.Errorf("saved, but apply failed: %w", err))
				}
				applied = true
			}

			emit(c, Envelope{OK: true, Data: map[string]any{
				"saved":    true,
				"applied":  applied,
				"settings": s.Adb,
			}})
			return nil
		},
	}

	usb = cmd.Flags().Bool("usb", true, "是否启用 USB 调试")
	tcp = cmd.Flags().Bool("tcpip", false, "是否启用网络调试")
	cmd.Flags().IntVar(&port, "port", 5555, "默认网络调试端口")
	boot = cmd.Flags().Bool("boot", false, "开机自动应用")
	cmd.Flags().BoolVar(&apply, "apply", false, "保存后立即应用到底层")

	return cmd
}

func newAdbApplyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "apply",
		Short: "根据当前已保存的设置立即应用生效",
		RunE: func(c *cobra.Command, _ []string) error {
			s, err := core.LoadSettings(paths())
			if err != nil {
				return fail(c, err)
			}
			ctrl := adb.New()
			if err := ctrl.Apply(context.Background(), s.Adb); err != nil {
				return fail(c, err)
			}
			st, _ := ctrl.GetStatus(context.Background())
			emit(c, Envelope{OK: true, Data: map[string]any{
				"applied":  true,
				"current":  st,
				"settings": s.Adb,
			}})
			return nil
		},
	}
}
