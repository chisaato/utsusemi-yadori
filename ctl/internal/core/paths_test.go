package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathsEnsureCreatesLayout(t *testing.T) {
	root := t.TempDir()
	p := New(filepath.Join(root, "utsusemi"))
	if err := p.Ensure(); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{p.Root, p.ServerDir(), p.GadgetDir(), p.Logs()} {
		if fi, err := os.Stat(d); err != nil || !fi.IsDir() {
			t.Fatalf("want dir %s", d)
		}
	}
}

func TestPathsFileLocations(t *testing.T) {
	p := New("/data/adb/utsusemi")
	want := map[string]string{
		p.Settings():     "/data/adb/utsusemi/settings.json",
		p.Manifest():     "/data/adb/utsusemi/manifest.json",
		p.Rules():        "/data/adb/utsusemi/rules.json",
		p.GadgetConfig(): "/data/adb/utsusemi/gadget.json",
		p.PidFile():      "/data/adb/utsusemi/frida-server.pid",
		p.ServerLog():    "/data/adb/utsusemi/logs/frida-server.log",
	}
	for got, w := range want {
		if got != w {
			t.Fatalf("got %s want %s", got, w)
		}
	}
	// 模块目录在数据根的兄弟 modules/utsusemi 下（可测试性）
	if got, want := p.ModuleProp(), "/data/adb/modules/utsusemi/module.prop"; got != want {
		t.Fatalf("ModuleProp got %s want %s", got, want)
	}
}
