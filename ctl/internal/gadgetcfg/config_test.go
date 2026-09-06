package gadgetcfg

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"utsusemi/ctl/internal/core"
)

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
	p := core.New(t.TempDir())
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
		len(t0.InjectedLibraries) != 1 || t0.InjectedLibraries[0].Path != p.GadgetDir()+"/official_1.0_arm64.so" {
		t.Fatalf("t0 %+v", t0)
	}
	if !c.Targets[1].ChildGating.Enabled || c.Targets[1].ChildGating.Mode != "freeze" {
		t.Fatalf("t1 %+v", c.Targets[1])
	}
}

func TestApplyEmptyRules(t *testing.T) {
	p := core.New(t.TempDir())
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
	p := core.New(t.TempDir())
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
