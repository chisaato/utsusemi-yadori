package binmgr

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"utsusemi/ctl/internal/core"
)

func elfFile(t *testing.T, dir, name string, class byte, et, machine uint16) string {
	t.Helper()
	h := make([]byte, 64)
	copy(h, []byte{0x7f, 'E', 'L', 'F'})
	h[4], h[5], h[6] = class, 1, 1
	binary.LittleEndian.PutUint16(h[16:], et)
	binary.LittleEndian.PutUint16(h[18:], machine)
	binary.LittleEndian.PutUint32(h[20:], 1) // e_version
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, h, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// hostClass/hostMachine 按设备规范架构给出 ELF 头参数
var hostClass = map[string]byte{"arm64": 2, "x86_64": 2, "arm": 1, "x86": 1}[core.DeviceArch()]
var hostMachine = map[string]uint16{"arm64": 183, "x86_64": 62, "arm": 40, "x86": 3}[core.DeviceArch()]

func newMgr(t *testing.T) *Manager {
	t.Helper()
	m, err := New(core.New(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestImportValidServerAndGadget(t *testing.T) {
	m := newMgr(t)
	src := t.TempDir()
	srv := elfFile(t, src, "my-server", hostClass, 2, hostMachine)   // ET_EXEC
	gad := elfFile(t, src, "my-gadget.so", hostClass, 3, hostMachine) // ET_DYN

	b, err := m.Import("server", srv, "")
	if err != nil {
		t.Fatal(err)
	}
	if b.File != "my-server" || b.Variant != "custom" || b.Version != "unknown" || b.ELFType != "exec" || b.Arch != core.DeviceArch() || b.SHA256 == "" {
		t.Fatalf("bad entry %+v", b)
	}
	if _, err := m.Import("gadget", gad, "mybuild-1"); err != nil {
		t.Fatal(err)
	}
	// 文件已复制到数据目录
	if _, err := os.Stat(filepath.Join(m.P.ServerDir(), "my-server")); err != nil {
		t.Fatal(err)
	}
}

func TestImportRejectsWrongTypeAndNonELF(t *testing.T) {
	m := newMgr(t)
	src := t.TempDir()
	script := filepath.Join(src, "plain")
	os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755)
	dynAsServer := elfFile(t, src, "dyn-file", hostClass, 3, hostMachine)

	if _, err := m.Import("server", script, ""); err == nil {
		t.Fatal("non-elf accepted")
	}
	if _, err := m.Import("server", dynAsServer, ""); err == nil {
		t.Fatal("dyn accepted as server")
	}
}

func TestImportNameConflictAppendsTimestamp(t *testing.T) {
	m := newMgr(t)
	src := t.TempDir()
	a := elfFile(t, src, "dup", hostClass, 2, hostMachine)
	b := elfFile(t, src, "dup", hostClass, 2, hostMachine)
	if _, err := m.Import("server", a, ""); err != nil {
		t.Fatal(err)
	}
	got, err := m.Import("server", b, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.File == "dup" {
		t.Fatal("conflict not deduped")
	}
	if len(got.File) <= len("dup") || got.File[:3] != "dup" {
		t.Fatalf("dedup name unexpected: %s", got.File)
	}
}

func TestRemoveActiveRefusedAndCleanup(t *testing.T) {
	m := newMgr(t)
	src := t.TempDir()
	a := elfFile(t, src, "keep", hostClass, 2, hostMachine)
	b := elfFile(t, src, "junk", hostClass, 2, hostMachine)
	ka, _ := m.Import("server", a, "")
	kb, _ := m.Import("server", b, "")
	if err := m.SetActive("server", ka.File); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove("server", ka.File); err == nil {
		t.Fatal("active removal allowed")
	}
	removed, err := m.Cleanup()
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != "server/"+kb.File {
		t.Fatalf("cleanup wrong: %v", removed)
	}
	if _, ok := m.M.Find("server", kb.File); ok {
		t.Fatal("cleanup left manifest entry")
	}
}

func TestSetActiveArchGuard(t *testing.T) {
	m := newMgr(t)
	src := t.TempDir()
	// 造一个非本机架构文件：arm64 机器上传 x86，反之亦然
	otherM := uint16(3)
	otherC := byte(1)
	if core.DeviceArch() == "arm" || core.DeviceArch() == "x86" {
		otherM, otherC = 183, 2
	}
	f := elfFile(t, src, "other", otherC, 2, otherM)
	b, err := m.Import("server", f, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.SetActive("server", b.File); err == nil {
		t.Fatal("arch mismatch activatable")
	}
}
