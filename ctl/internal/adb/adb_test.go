package adb

import (
	"context"
	"testing"

	"utsusemi/ctl/internal/core"
)

type mockRunner struct {
	commands [][]string
	props    map[string]string
	settings map[string]string
}

func newMockRunner() *mockRunner {
	return &mockRunner{
		props: map[string]string{
			"init.svc.adbd":         "running",
			"service.adb.tcp.port": "5555",
		},
		settings: map[string]string{
			"global:adb_enabled": "1",
		},
	}
}

func (m *mockRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := append([]string{name}, args...)
	m.commands = append(m.commands, cmd)

	if name == "getprop" && len(args) > 0 {
		return m.props[args[0]], nil
	}
	if name == "setprop" && len(args) >= 2 {
		m.props[args[0]] = args[1]
		return "", nil
	}
	if name == "settings" && len(args) >= 3 && args[0] == "get" {
		key := args[1] + ":" + args[2]
		return m.settings[key], nil
	}
	if name == "settings" && len(args) >= 4 && args[0] == "put" {
		key := args[1] + ":" + args[2]
		m.settings[key] = args[3]
		return "", nil
	}
	return "", nil
}

func TestAdbGetStatus(t *testing.T) {
	runner := newMockRunner()
	ctrl := NewWithRunner(runner)

	st, err := ctrl.GetStatus(context.Background())
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if !st.AdbdRunning {
		t.Errorf("expected adbd running")
	}
	if !st.UsbEnabled {
		t.Errorf("expected usb enabled")
	}
	if !st.TcpipEnabled || st.TcpipPort != 5555 {
		t.Errorf("expected tcpip enabled with port 5555, got enabled=%v, port=%d", st.TcpipEnabled, st.TcpipPort)
	}
}

func TestAdbSetUsb(t *testing.T) {
	runner := newMockRunner()
	ctrl := NewWithRunner(runner)

	// 关闭 USB 调试
	if err := ctrl.SetUsb(context.Background(), false); err != nil {
		t.Fatalf("SetUsb false failed: %v", err)
	}
	if runner.settings["global:adb_enabled"] != "0" {
		t.Errorf("expected adb_enabled 0, got %s", runner.settings["global:adb_enabled"])
	}

	// 开启 USB 调试
	if err := ctrl.SetUsb(context.Background(), true); err != nil {
		t.Fatalf("SetUsb true failed: %v", err)
	}
	if runner.settings["global:adb_enabled"] != "1" {
		t.Errorf("expected adb_enabled 1, got %s", runner.settings["global:adb_enabled"])
	}
}

func TestAdbSetTcpip(t *testing.T) {
	runner := newMockRunner()
	ctrl := NewWithRunner(runner)

	// 开启并指定端口 6666
	if err := ctrl.SetTcpip(context.Background(), true, 6666); err != nil {
		t.Fatalf("SetTcpip failed: %v", err)
	}
	if runner.props["service.adb.tcp.port"] != "6666" {
		t.Errorf("expected port 6666, got %s", runner.props["service.adb.tcp.port"])
	}

	// 关闭网络调试
	if err := ctrl.SetTcpip(context.Background(), false, 0); err != nil {
		t.Fatalf("SetTcpip disable failed: %v", err)
	}
	if runner.props["service.adb.tcp.port"] != "-1" {
		t.Errorf("expected port -1, got %s", runner.props["service.adb.tcp.port"])
	}
}

func TestAdbApply(t *testing.T) {
	runner := newMockRunner()
	ctrl := NewWithRunner(runner)

	s := core.AdbSettings{
		UsbEnabled:   true,
		TcpipEnabled: true,
		Port:         5555,
		ApplyOnBoot:  true,
	}

	if err := ctrl.Apply(context.Background(), s); err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if runner.settings["global:adb_enabled"] != "1" {
		t.Errorf("expected adb_enabled 1")
	}
	if runner.props["service.adb.tcp.port"] != "5555" {
		t.Errorf("expected port 5555")
	}
}
