package core

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestLoadSettingsMissingReturnsDefaults(t *testing.T) {
	p := New(t.TempDir())
	s, err := LoadSettings(p)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Server.Autostart || s.Server.Active != "" || s.Download.GithubAPI != "https://api.github.com" {
		t.Fatalf("unexpected defaults: %+v", s)
	}
}

func TestSaveSettingsRoundTripAtomic(t *testing.T) {
	p := New(t.TempDir())
	s := DefaultSettings()
	s.Server.Active = "official_17.2.14_arm64"
	s.Server.Args = []string{"-l", "0.0.0.0:27042"}
	s.Gadget.Active = "official_17.2.14_arm64.so"
	if err := SaveSettings(p, s); err != nil {
		t.Fatal(err)
	}
	// 原子性：无残留 tmp 文件且 settings.json 存在
	ents, _ := os.ReadDir(p.Root)
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("tmp leftover: %s", e.Name())
		}
	}
	if _, err := os.Stat(p.Settings()); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSettings(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Server.Active != s.Server.Active || len(got.Server.Args) != 2 || got.Gadget.Active != s.Gadget.Active {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	if _, err := json.Marshal(got); err != nil { // 可序列化
		t.Fatal(err)
	}
}
