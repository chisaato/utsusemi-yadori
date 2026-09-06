package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"utsusemi/ctl/internal/core"
)

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
	// 默认 autostart=true，但无激活二进制 → server 不运行，命令仍成功
	if !d.Autostart || d.Server.Running {
		t.Fatalf("boot data: %+v", d)
	}
	// 关闭 autostart 后 boot 直接跳过
	runJSON(t, "--data-root", root, "server", "set", "--autostart=false")
	env = runJSON(t, "--data-root", root, "boot")
	raw, _ = json.Marshal(env.Data)
	json.Unmarshal(raw, &d)
	if d.Autostart {
		t.Fatal("autostart off but boot attempted start")
	}
}
