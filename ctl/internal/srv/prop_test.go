package srv

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"utsusemi/ctl/internal/core"
)

func TestUpdateDescription(t *testing.T) {
	root := t.TempDir()
	// 数据根 root/utsusemi，模块目录在 root/modules/utsusemi
	p := core.New(filepath.Join(root, "utsusemi"))
	modDir := filepath.Join(root, "modules", "utsusemi")
	os.MkdirAll(modDir, 0o755)
	prop := filepath.Join(modDir, "module.prop")
	os.WriteFile(prop, []byte("id=utsusemi\nname=Utsusemi\ndescription=old\n"), 0o644)

	if err := UpdateDescription(p, true, ""); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(prop)
	if !strings.Contains(string(raw), "description=frida-server: ● running") {
		t.Fatalf("prop not updated: %s", raw)
	}
	// module.prop 缺失时静默跳过
	p2 := core.New(filepath.Join(root, "other"))
	if err := UpdateDescription(p2, false, ""); err != nil {
		t.Fatal(err)
	}
}
