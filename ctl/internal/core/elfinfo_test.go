package core

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

// craft 构建 ELF e_ident+头部：class 1=32bit 2=64bit；et 2=EXEC 3=DYN；machine 见常量
func craft(class byte, et, machine uint16) *bytes.Reader {
	h := make([]byte, 64)
	copy(h, []byte{0x7f, 'E', 'L', 'F'})
	h[4], h[5], h[6] = class, 1, 1 // class, LE, EV_CURRENT
	binary.LittleEndian.PutUint16(h[16:], et)
	binary.LittleEndian.PutUint16(h[18:], machine)
	// e_version 字段（偏移 20）也须为 EV_CURRENT(1)
	binary.LittleEndian.PutUint32(h[20:], 1)
	return bytes.NewReader(h)
}

const (
	EM_386     = 3
	EM_ARM     = 40
	EM_X86_64  = 62
	EM_AARCH64 = 183
)

func TestInspectArchAndType(t *testing.T) {
	cases := []struct {
		class byte
		et, m uint16
		arch  string
		typ   string
	}{
		{2, 2, EM_AARCH64, "arm64", "exec"},
		{2, 3, EM_AARCH64, "arm64", "dyn"},
		{1, 2, EM_ARM, "arm", "exec"},
		{1, 3, EM_ARM, "arm", "dyn"},
		{2, 2, EM_X86_64, "x86_64", "exec"},
		{1, 2, EM_386, "x86", "exec"},
		{2, 2, 9999, "unknown", "exec"},
		{2, 4, EM_AARCH64, "arm64", "unknown"}, // ET_CORE 等其他类型
	}
	for _, c := range cases {
		info, err := Inspect(craft(c.class, c.et, c.m))
		if err != nil {
			t.Fatalf("class=%d et=%d m=%d: %v", c.class, c.et, c.m, err)
		}
		if info.Arch != c.arch || info.Type != c.typ {
			t.Fatalf("got %+v want arch=%s type=%s", info, c.arch, c.typ)
		}
	}
}

func TestInspectRejectsNonELF(t *testing.T) {
	if _, err := Inspect(bytes.NewReader([]byte("#!/bin/sh\nexit 0\n"))); err == nil || !strings.Contains(err.Error(), "not an ELF") {
		t.Fatalf("want not-an-ELF error, got %v", err)
	}
}

func TestInspectFileRealHostBinary(t *testing.T) {
	// /bin/sh 在 linux 上必为 ELF；取其类型断言为 exec 或 dyn 均合法
	info, err := InspectFile("/bin/sh")
	if err != nil {
		t.Skip("host /bin/sh not ELF (non-linux)")
	}
	if info.Type != "exec" && info.Type != "dyn" {
		t.Fatalf("unexpected type %+v", info)
	}
}
