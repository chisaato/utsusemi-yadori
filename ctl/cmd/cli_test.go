package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"utsusemi/ctl/internal/core"
	"utsusemi/ctl/internal/srv"
)

// procAlive 判断进程是否真正存活（zombie 视为已终止）
func procAlive(pid int) bool {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false
	}
	s := string(raw)
	i := strings.LastIndex(s, ")")
	if i < 0 || i+2 >= len(s) {
		return false
	}
	return s[i+2] != 'Z'
}

func runJSON(t *testing.T, args ...string) Envelope {
	t.Helper()
	var out, errb bytes.Buffer
	code := Execute(append(args, "--json"), &out, &errb)
	var env Envelope
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("bad json %q (stderr %q): %v", out.String(), errb.String(), err)
	}
	if env.OK != (code == 0) {
		t.Fatalf("code=%d env=%+v stderr=%q", code, env, errb.String())
	}
	return env
}

// hostExecELF 造一个与设备架构匹配的 ET_EXEC 文件
func hostExecELF(t *testing.T, path string) {
	t.Helper()
	class := byte(2)
	machine := uint16(183)
	switch core.DeviceArch() {
	case "arm":
		class, machine = 1, 40
	case "x86_64":
		class, machine = 2, 62
	case "x86":
		class, machine = 1, 3
	}
	h := make([]byte, 64)
	copy(h, []byte{0x7f, 'E', 'L', 'F'})
	h[4], h[5], h[6] = class, 1, 1
	h[16], h[17] = 2, 0 // ET_EXEC
	h[18], h[19] = byte(machine), byte(machine>>8)
	h[20] = 1 // e_version
	if err := os.WriteFile(path, h, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestBinListImportUse(t *testing.T) {
	root := t.TempDir()
	with := func(args ...string) []string { return append([]string{"--data-root", root}, args...) }

	env := runJSON(t, with("bin", "list")...)
	if !env.OK {
		t.Fatal(env.Error)
	}
	var bins []core.Binary
	raw, _ := json.Marshal(env.Data)
	json.Unmarshal(raw, &bins)
	if len(bins) != 0 {
		t.Fatalf("want empty, got %v", bins)
	}

	src := filepath.Join(root, "myserver")
	hostExecELF(t, src)
	env = runJSON(t, with("bin", "import", "--type", "server", "--file", src)...)
	if !env.OK {
		t.Fatal(env.Error)
	}
	env = runJSON(t, with("bin", "use", "--type", "server", "--file", "myserver")...)
	if !env.OK {
		t.Fatal(env.Error)
	}
	// exec ELF 不能当 gadget 导入
	env = runJSON(t, with("bin", "import", "--type", "gadget", "--file", src)...)
	if env.OK {
		t.Fatal("exec ELF accepted as gadget")
	}
}

func TestGadgetSetAndApply(t *testing.T) {
	root := t.TempDir()
	with := func(args ...string) []string { return append([]string{"--data-root", root}, args...) }

	env := runJSON(t, with("gadget", "set", "--app", "com.x", "--enable", "--delay", "100")...)
	if !env.OK {
		t.Fatal(env.Error)
	}
	env = runJSON(t, with("gadget", "rules")...)
	if !env.OK {
		t.Fatal(env.Error)
	}
	raw, _ := json.Marshal(env.Data)
	var rr struct {
		Rules []map[string]any
	}
	json.Unmarshal(raw, &rr)
	if len(rr.Rules) != 1 || rr.Rules[0]["app_name"] != "com.x" {
		t.Fatalf("rules %+v", rr)
	}
	// 无 gadget 时 apply 应失败并给出清晰错误
	env = runJSON(t, with("gadget", "apply")...)
	if env.OK || !strings.Contains(env.Error, "gadget") {
		t.Fatalf("apply without gadget: %+v", env)
	}
}

func TestServerStatusNoBinary(t *testing.T) {
	root := t.TempDir()
	env := runJSON(t, "--data-root", root, "server", "status")
	if !env.OK {
		t.Fatal(env.Error)
	}
	raw, _ := json.Marshal(env.Data)
	var st map[string]any
	json.Unmarshal(raw, &st)
	if st["running"] != false {
		t.Fatalf("st %+v", st)
	}
}

func TestStatusSummary(t *testing.T) {
	root := t.TempDir()
	env := runJSON(t, "--data-root", root, "status")
	if !env.OK {
		t.Fatal(env.Error)
	}
	raw, _ := json.Marshal(env.Data)
	var st map[string]any
	json.Unmarshal(raw, &st)
	for _, k := range []string{"server", "servers", "gadgets", "rules", "web"} {
		if _, ok := st[k]; !ok {
			t.Fatalf("status missing %q: %v", k, st)
		}
	}
}

func TestBootRespectsAutostart(t *testing.T) {
	root := t.TempDir()
	env := runJSON(t, "--data-root", root, "boot")
	if !env.OK {
		t.Fatal(env.Error)
	}
	raw, _ := json.Marshal(env.Data)
	var d struct {
		Autostart bool `json:"autostart"`
		Server    struct {
			Running bool `json:"running"`
		} `json:"server"`
	}
	json.Unmarshal(raw, &d)
	// 默认 autostart=false，server 不运行
	if d.Autostart || d.Server.Running {
		t.Fatalf("boot data: %+v", d)
	}
	// 开启 autostart 后（但无激活二进制）→ autostart=true，仍不运行
	runJSON(t, "--data-root", root, "server", "set", "--autostart=true")
	env = runJSON(t, "--data-root", root, "boot")
	raw, _ = json.Marshal(env.Data)
	json.Unmarshal(raw, &d)
	if !d.Autostart || d.Server.Running {
		t.Fatalf("autostart on but unexpected state: %+v", d)
	}
}

func TestAdbCLIAndAPI(t *testing.T) {
	root := t.TempDir()

	// adb status (json)
	env := runJSON(t, "--data-root", root, "adb", "status")
	if !env.OK {
		t.Fatalf("adb status: %v", env.Error)
	}

	// adb set
	env = runJSON(t, "--data-root", root, "adb", "set", "--usb=true", "--tcpip=true", "--port=5556", "--boot=true")
	if !env.OK {
		t.Fatalf("adb set: %v", env.Error)
	}

	s, err := core.LoadSettings(core.New(root))
	if err != nil {
		t.Fatal(err)
	}
	if !s.Adb.UsbEnabled || !s.Adb.TcpipEnabled || s.Adb.Port != 5556 || !s.Adb.ApplyOnBoot {
		t.Fatalf("adb settings mismatch: %+v", s.Adb)
	}

	// api adb/status
	var out, errb bytes.Buffer
	code := Execute([]string{"--data-root", root, "api", "adb/status"}, &out, &errb)
	if code != 0 {
		t.Fatalf("api adb/status failed: %s", errb.String())
	}
	var apiEnv Envelope
	if err := json.Unmarshal(out.Bytes(), &apiEnv); err != nil || !apiEnv.OK {
		t.Fatalf("api adb/status env: %+v", apiEnv)
	}
}

// startOldServer 登记并启动一个可真实运行的 shell 脚本核心，返回 paths 与该进程 PID
func startOldServer(t *testing.T, root string) (core.Paths, int) {
	t.Helper()
	p := core.New(root)
	if err := p.Ensure(); err != nil {
		t.Fatal(err)
	}
	oldFile := "oldserver"
	if err := os.WriteFile(filepath.Join(p.ServerDir(), oldFile), []byte("#!/bin/sh\nsleep 60\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	man := core.Manifest{}
	man.Upsert("server", core.Binary{
		File: oldFile, Variant: "custom", Version: "old",
		Arch: core.DeviceArch(), ELFType: "exec", AddedAt: time.Now(),
	})
	if err := core.SaveManifest(p, man); err != nil {
		t.Fatal(err)
	}
	s, err := core.LoadSettings(p)
	if err != nil {
		t.Fatal(err)
	}
	s.Server.Active = oldFile
	if err := core.SaveSettings(p, s); err != nil {
		t.Fatal(err)
	}
	st, err := srv.New(p).Start()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Running || st.PID <= 0 {
		t.Fatalf("old server not running: %+v", st)
	}
	return p, st.PID
}

// addSwitchTarget 写入一个合法 ELF 并登记为新的 server 核心（不启动）
func addSwitchTarget(t *testing.T, p core.Paths, file string) {
	t.Helper()
	hostExecELF(t, filepath.Join(p.ServerDir(), file))
	man, err := core.LoadManifest(p)
	if err != nil {
		t.Fatal(err)
	}
	man.Upsert("server", core.Binary{
		File: file, Variant: "custom", Version: "new",
		Arch: core.DeviceArch(), ELFType: "exec", AddedAt: time.Now(),
	})
	if err := core.SaveManifest(p, man); err != nil {
		t.Fatal(err)
	}
}

// TestBinUseStopsPreviousServer 验证 ops.BinUse 切换核心时旧进程被终止、
// pidfile 被清理、新核心不会被自动启动
func TestBinUseStopsPreviousServer(t *testing.T) {
	root := t.TempDir()
	p, oldPID := startOldServer(t, root)
	addSwitchTarget(t, p, "newserver")

	data, err := opsFor(p).BinUse("server", "newserver")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(data)
	var res struct {
		Active          string `json:"active"`
		StoppedPrevious bool   `json:"stopped_previous"`
	}
	json.Unmarshal(raw, &res)
	if res.Active != "newserver" || !res.StoppedPrevious {
		t.Fatalf("BinUse data: %s", raw)
	}
	if procAlive(oldPID) {
		t.Fatalf("old server pid %d still alive", oldPID)
	}
	if _, err := os.Stat(p.PidFile()); !os.IsNotExist(err) {
		t.Fatalf("pidfile not cleaned: %v", err)
	}
	if got := srv.New(p).Status(); got.Running {
		t.Fatalf("new core must not auto-start: %+v", got)
	}
	s2, err := core.LoadSettings(p)
	if err != nil {
		t.Fatal(err)
	}
	if s2.Server.Active != "newserver" {
		t.Fatalf("active not switched: %q", s2.Server.Active)
	}
}

// TestCLIBinUseStopsPreviousServer 覆盖 bin use 命令路径的旧核心终止行为
func TestCLIBinUseStopsPreviousServer(t *testing.T) {
	root := t.TempDir()
	p, oldPID := startOldServer(t, root)
	addSwitchTarget(t, p, "newserver")

	env := runJSON(t, "--data-root", root, "bin", "use", "--type", "server", "--file", "newserver")
	if !env.OK {
		t.Fatal(env.Error)
	}
	if procAlive(oldPID) {
		t.Fatalf("old server pid %d still alive after CLI bin use", oldPID)
	}
	if _, err := os.Stat(p.PidFile()); !os.IsNotExist(err) {
		t.Fatalf("pidfile not cleaned: %v", err)
	}
	if got := srv.New(p).Status(); got.Running {
		t.Fatalf("new core must not auto-start: %+v", got)
	}
}
