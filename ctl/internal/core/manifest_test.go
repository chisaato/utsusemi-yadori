package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func mkFile(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestManifestRoundTripAndUpsert(t *testing.T) {
	p := New(t.TempDir())
	m := Manifest{}
	m.Upsert("server", Binary{File: "a", Variant: "official", Version: "1.0", Arch: "arm64", ELFType: "exec", Size: 3, AddedAt: time.Now()})
	m.Upsert("server", Binary{File: "a", Version: "1.1"}) // 同名覆盖
	if err := SaveManifest(p, m); err != nil {
		t.Fatal(err)
	}
	got, err := LoadManifest(p)
	if err != nil {
		t.Fatal(err)
	}
	b, ok := got.Find("server", "a")
	if !ok || b.Version != "1.1" || b.Variant != "official" {
		t.Fatalf("upsert/roundtrip wrong: %+v", b)
	}
	if _, ok := got.Find("gadget", "a"); ok {
		t.Fatal("server entry leaked into gadget")
	}
}

func TestReconcileOrphanAndMissing(t *testing.T) {
	p := New(t.TempDir())
	if err := p.Ensure(); err != nil {
		t.Fatal(err)
	}
	mkFile(t, p.ServerDir(), "frida-server-test") // 孤儿：不在 manifest
	mkFile(t, p.GadgetDir(), "official_1.0_arm64.so")

	m := Manifest{}
	m.Upsert("gadget", Binary{File: "gone.so", Variant: "official", Arch: "arm64", ELFType: "dyn"}) // 缺失项
	m.Upsert("gadget", Binary{File: "official_1.0_arm64.so", Variant: "official", Arch: "arm64", ELFType: "dyn"})

	Reconcile(p, &m)

	// 孤儿被登记为 custom/unknown
	if b, ok := m.Find("server", "frida-server-test"); !ok || b.Variant != "custom" || b.Version != "unknown" || b.Arch != "unknown" {
		t.Fatalf("orphan not registered: %+v", b)
	}
	// 缺失标记
	if b, _ := m.Find("gadget", "gone.so"); !b.Missing {
		t.Fatal("missing flag not set")
	}
	// 存在项不标记
	if b, _ := m.Find("gadget", "official_1.0_arm64.so"); b.Missing {
		t.Fatal("existing flagged missing")
	}
}
