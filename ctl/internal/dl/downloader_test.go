package dl

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ulikunitz/xz"

	"utsusemi/ctl/internal/core"
)

// elfPayload 造一个与设备架构匹配的最小 ELF 头
func elfPayload(t *testing.T) []byte {
	t.Helper()
	class := byte(2)
	machine := uint16(183)
	et := uint16(2)
	switch core.DeviceArch() {
	case "arm":
		class, machine, et = 1, 40, 2
	case "x86_64":
		class, machine = 2, 62
	case "x86":
		class, machine, et = 1, 3, 2
	}
	h := make([]byte, 64)
	copy(h, []byte{0x7f, 'E', 'L', 'F'})
	h[4], h[5], h[6] = class, 1, 1
	h[16] = byte(et)
	h[17] = byte(et >> 8)
	h[18] = byte(machine)
	h[19] = byte(machine >> 8)
	// e_version
	h[20], h[21], h[22], h[23] = 1, 0, 0, 0
	return h
}

func xzBytes(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := xz.NewWriter(&buf)
	if err != nil {
		t.Fatal(err)
	}
	w.Write(raw)
	w.Close()
	return buf.Bytes()
}

func gzBytes(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	w.Write(raw)
	w.Close()
	return buf.Bytes()
}

func TestDecompressXzAndGz(t *testing.T) {
	dir := t.TempDir()
	raw := elfPayload(t)

	xf := filepath.Join(dir, "a.xz")
	os.WriteFile(xf, xzBytes(t, raw), 0o644)
	got, err := Decompress(xf, ".xz")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(dir, "a") {
		t.Fatalf("got %s", got)
	}
	data, _ := os.ReadFile(got)
	if !bytes.Equal(data, raw) {
		t.Fatal("xz roundtrip mismatch")
	}
	if _, err := os.Stat(xf); !os.IsNotExist(err) {
		t.Fatal("compressed leftover")
	}

	gf := filepath.Join(dir, "b.so.gz")
	os.WriteFile(gf, gzBytes(t, raw), 0o644)
	got2, err := Decompress(gf, ".gz")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(got2) != "b.so" {
		t.Fatalf("got %s", got2)
	}
}

func TestInstallFullFlow(t *testing.T) {
	p := core.New(t.TempDir())
	p.Ensure()
	raw := elfPayload(t)

	mux := http.NewServeMux()
	// ghproxy 风格镜像：请求路径为 前缀+完整原始 URL
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/server.xz") {
			w.Write(xzBytes(t, raw))
			return
		}
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	a := Asset{Name: "frida-server-17.2.14-android-" + core.DeviceArch() + ".xz",
		URL: "https://github.com/frida/frida/releases/download/17.2.14/server.xz",
		Version: "17.2.14", Arch: core.DeviceArch(), BinType: "server", Size: int64(len(raw))}

	s := core.DefaultSettings()
	s.Download = core.DownloadSettings{GithubAPI: srv.URL, Mirror: srv.URL + "/dl/"}

	m := core.Manifest{}
	b, err := Install(context.Background(), p, s, &m, "official", a, nil)
	if err != nil {
		t.Fatal(err)
	}
	if b.File != "official_17.2.14_"+core.DeviceArch() || b.Variant != "official" || b.ELFType != "exec" || b.SHA256 == "" || b.Source == "" {
		t.Fatalf("bad entry %+v", b)
	}
	st, err := os.Stat(filepath.Join(p.ServerDir(), b.File))
	if err != nil || st.Mode().Perm()&0o111 == 0 {
		t.Fatalf("server not executable: %v %v", st, err)
	}
	if _, ok := m.Find("server", b.File); !ok {
		t.Fatal("manifest not updated")
	}
}

func TestInstallRejectsArchMismatch(t *testing.T) {
	p := core.New(t.TempDir())
	p.Ensure()
	// 造 x86_64 ELF 头但资产声称其他架构
	h := make([]byte, 64)
	copy(h, []byte{0x7f, 'E', 'L', 'F'})
	h[4], h[5], h[6] = 2, 1, 1
	h[18], h[19] = 62, 0 // EM_X86_64
	mux := http.NewServeMux()
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/s.gz") {
			w.Write(gzBytes(t, h))
			return
		}
		http.NotFound(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	claimed := "arm"
	if core.DeviceArch() != "x86_64" {
		claimed = "x86_64"
	}
	a := Asset{Name: "x.gz", URL: "https://github.com/x/s.gz", Version: "1.0", Arch: claimed, BinType: "gadget", Size: 64}
	s := core.DefaultSettings()
	s.Download = core.DownloadSettings{GithubAPI: srv.URL, Mirror: srv.URL + "/dl/"}
	m := core.Manifest{}
	if _, err := Install(context.Background(), p, s, &m, "florida", a, nil); err == nil {
		t.Fatal("arch mismatch accepted")
	}
}
