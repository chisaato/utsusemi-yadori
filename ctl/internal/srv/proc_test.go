package srv

import (
	"os"
	"path/filepath"
	"runtime"
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
