package srv

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"utsusemi/ctl/internal/core"
)

// fakeServer 写一个可执行 sleep 脚本并登记 manifest（srv 信任 manifest，导入时已做 ELF 验证）
func fakeServer(t *testing.T, p core.Paths) core.Binary {
	t.Helper()
	// 不用 exec：保留 cmdline 含脚本路径，供 alive() 的文件名校验
	script := "#!/system/bin/sh\nsleep 60\n"
	if runtime.GOOS != "android" {
		script = "#!/bin/sh\nsleep 60\n"
	}
	f := filepath.Join(p.ServerDir(), "fakeserver")
	if err := os.WriteFile(f, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	b := core.Binary{File: "fakeserver", Variant: "custom", Version: "test", Arch: core.DeviceArch(), ELFType: "exec", AddedAt: time.Now()}
	m := core.Manifest{}
	m.Upsert("server", b)
	if err := core.SaveManifest(p, m); err != nil {
		t.Fatal(err)
	}
	s, _ := core.LoadSettings(p)
	s.Server.Active = "fakeserver"
	s.Server.Autostart = false
	if err := core.SaveSettings(p, s); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestStartStopStatus(t *testing.T) {
	p := core.New(t.TempDir())
	if err := p.Ensure(); err != nil {
		t.Fatal(err)
	}
	fakeServer(t, p)
	m := New(p)

	st, err := m.Start()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Running || st.PID <= 0 {
		t.Fatalf("start: %+v", st)
	}
	// 幂等
	st2, err := m.Start()
	if err != nil || !st2.Running || st2.PID != st.PID {
		t.Fatalf("idempotent start: %+v err=%v", st2, err)
	}
	if got := m.Status(); !got.Running {
		t.Fatalf("status: %+v", got)
	}
	if _, err := m.Stop(); err != nil {
		t.Fatal(err)
	}
	if got := m.Status(); got.Running {
		t.Fatalf("stopped: %+v", got)
	}
	// 再 Stop 幂等
	if _, err := m.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestStartWithoutActiveBinary(t *testing.T) {
	p := core.New(t.TempDir())
	p.Ensure()
	if _, err := New(p).Start(); err == nil {
		t.Fatal("start without active should fail")
	}
}

func TestParseListenPort(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"default", nil, 27042},
		{"short", []string{"-l", "127.0.0.1:28042"}, 28042},
		{"long equals", []string{"--listen=0.0.0.0:29042"}, 29042},
		{"colon only", []string{"-l", ":30042"}, 30042},
		{"bare port", []string{"-l", "31042"}, 31042},
		{"invalid", []string{"-l", "not-a-port"}, 27042},
		{"zero", []string{"-l", "127.0.0.1:0"}, 27042},
		{"out of range", []string{"-l", "127.0.0.1:70000"}, 27042},
		{"missing value", []string{"-l"}, 27042},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseListenPort(tc.args); got != tc.want {
				t.Fatalf("parseListenPort(%v)=%d want %d", tc.args, got, tc.want)
			}
		})
	}
}

// TestKillResidualsStopsServer 验证残留清理会终止 pidfile 记录的进程、
// 清空 pidfile 并把 module.prop 标记为 stopped
func TestKillResidualsStopsServer(t *testing.T) {
	p := core.New(t.TempDir())
	if err := p.Ensure(); err != nil {
		t.Fatal(err)
	}
	fakeServer(t, p)
	m := New(p)

	st, err := m.Start()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Running || st.PID <= 0 {
		t.Fatalf("start: %+v", st)
	}
	// 预置 module.prop 以验证状态回写
	if err := os.MkdirAll(filepath.Dir(p.ModuleProp()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.ModuleProp(), []byte("id=utsusemi\ndescription=frida-server: ● running\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	killed, err := KillResiduals(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(killed) == 0 {
		t.Fatal("expected residual pid to be killed")
	}
	if alive(st.PID, "fakeserver") {
		t.Fatalf("pid %d still alive after KillResiduals", st.PID)
	}
	if _, err := os.Stat(p.PidFile()); !os.IsNotExist(err) {
		t.Fatalf("pidfile not removed: %v", err)
	}
	raw, err := os.ReadFile(p.ModuleProp())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "○ stopped") {
		t.Fatalf("module.prop not stopped: %s", raw)
	}
	if got := m.Status(); got.Running {
		t.Fatalf("status still running: %+v", got)
	}
}
