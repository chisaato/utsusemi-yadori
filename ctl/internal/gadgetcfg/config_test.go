package gadgetcfg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"utsusemi/ctl/internal/core"
)

func newStagePaths(t *testing.T) core.Paths {
	t.Helper()
	return core.NewStage(t.TempDir(), filepath.Join(t.TempDir(), "stage"))
}

func seedGadget(t *testing.T, p core.Paths, file string) {
	t.Helper()
	os.WriteFile(p.GadgetDir()+"/"+file, []byte("fake-so"), 0o644)
	m := core.Manifest{}
	m.Upsert("gadget", core.Binary{File: file, Variant: "official", Version: "1.0",
		Arch: "arm64", ELFType: "dyn", AddedAt: time.Now()})
	core.SaveManifest(p, m)
	s, _ := core.LoadSettings(p)
	s.Gadget.Active = file
	core.SaveSettings(p, s)
}

func TestApplyGeneratesZygiskFridaCompatibleConfig(t *testing.T) {
	p := newStagePaths(t)
	p.Ensure()
	seedGadget(t, p, "official_1.0_arm64.so")

	r := Rules{Rules: []Rule{
		{AppName: "com.a", Enabled: true, StartUpDelayMS: 150},
		{AppName: "com.b", Enabled: false},
		{AppName: "com.c", Enabled: true, ChildGatingEnabled: true, ChildGatingMode: "freeze"},
	}}
	if err := r.Save(p); err != nil {
		t.Fatal(err)
	}
	if err := Apply(p); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p.GadgetConfig())
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	if len(c.Targets) != 2 { // com.b 被 disabled 排除
		t.Fatalf("targets %+v", c.Targets)
	}
	t0 := c.Targets[0]
	if t0.AppName != "com.a" || !t0.Enabled || t0.StartUpDelayMS != 150 ||
		len(t0.InjectedLibraries) != 1 || t0.InjectedLibraries[0].Path != filepath.Join(p.Stage, "official_1.0_arm64.so") {
		t.Fatalf("t0 %+v", t0)
	}
	if !c.Targets[1].ChildGating.Enabled || c.Targets[1].ChildGating.Mode != "freeze" {
		t.Fatalf("t1 %+v", c.Targets[1])
	}
}

func TestApplyEmptyRules(t *testing.T) {
	p := newStagePaths(t)
	p.Ensure()
	if err := Apply(p); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p.GadgetConfig())
	if string(raw) != "{\n  \"targets\": []\n}" {
		t.Fatalf("empty rules should emit empty targets, got %s", raw)
	}
}

func TestApplyFailsWhenGadgetMissing(t *testing.T) {
	p := newStagePaths(t)
	p.Ensure()
	s, _ := core.LoadSettings(p)
	s.Gadget.Active = "ghost.so"
	core.SaveSettings(p, s)
	r := Rules{Rules: []Rule{{AppName: "com.a", Enabled: true}}}
	r.Save(p)
	if err := Apply(p); err == nil {
		t.Fatal("missing gadget should fail apply")
	}
}

func TestListAppsViaFakePm(t *testing.T) {
	run := func(name string, args ...string) (string, error) {
		if name == "pm" && len(args) == 3 && args[0] == "list" && args[2] == "-3" {
			return "package:com.user1\npackage:com.user2\n", nil
		}
		if name == "pm" && args[0] == "list" {
			return "package:com.user1\npackage:com.user2\npackage:com.sys\n", nil
		}
		return "", nil
	}
	apps, err := ListApps(run)
	if err != nil {
		t.Fatal(err)
	}
	byPkg := map[string]App{}
	for _, a := range apps {
		byPkg[a.Package] = a
	}
	if len(apps) != 3 || !byPkg["com.sys"].System || byPkg["com.user1"].System {
		t.Fatalf("apps %+v", apps)
	}
}

func TestApplyPublishesGadgetToStage(t *testing.T) {
	p := newStagePaths(t)
	p.Ensure()
	seedGadget(t, p, "official_1.0_arm64.so")
	r := Rules{Rules: []Rule{{AppName: "com.a", Enabled: true}}}
	if err := r.Save(p); err != nil {
		t.Fatal(err)
	}
	if err := Apply(p); err != nil {
		t.Fatal(err)
	}
	staged := filepath.Join(p.Stage, "official_1.0_arm64.so")
	if _, err := os.Stat(staged); err != nil {
		t.Fatal("gadget not published to stage")
	}
	raw, _ := os.ReadFile(p.GadgetConfig())
	var c Config
	json.Unmarshal(raw, &c)
	if c.Targets[0].InjectedLibraries[0].Path != staged {
		t.Fatalf("path should point to stage: %+v", c.Targets[0])
	}
	// 二次 apply（内容相同）幂等不报错
	if err := Apply(p); err != nil {
		t.Fatal(err)
	}
	// 源更新后 apply 应刷新发布区（发布区不是事实来源）
	os.WriteFile(filepath.Join(p.GadgetDir(), "official_1.0_arm64.so"), []byte("fake-so-v2"), 0o644)
	if err := Apply(p); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(staged)
	if string(got) != "fake-so-v2" {
		t.Fatalf("stage not refreshed: %q", string(got))
	}
}
