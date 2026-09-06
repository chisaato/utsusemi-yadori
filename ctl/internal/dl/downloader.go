package dl

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ulikunitz/xz"

	"utsusemi/ctl/internal/core"
)

// Fetch 流式下载到 w，返回 sha256
func Fetch(ctx context.Context, c *Client, url string, w io.Writer) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "utsusemi-ctl")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: %s", url, resp.Status)
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(w, h), resp.Body); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Decompress 解压 .xz/.gz 为去后缀文件并删原包
func Decompress(path, ext string) (string, error) {
	in, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer in.Close()
	outPath := strings.TrimSuffix(path, ext)
	out, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	var r io.Reader
	switch ext {
	case ".xz":
		r, err = xz.NewReader(in)
	case ".gz":
		r, err = gzip.NewReader(in)
	default:
		err = fmt.Errorf("unsupported ext %q", ext)
	}
	if err == nil {
		_, err = io.Copy(out, r)
	}
	closeErr := out.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	return outPath, nil
}

// Install 下载→解压→终验→登记；覆盖同名（视为重下载）
func Install(ctx context.Context, p core.Paths, s core.Settings, m *core.Manifest, variant string, a Asset) (core.Binary, error) {
	dir := p.ServerDir()
	name := fmt.Sprintf("%s_%s_%s", variant, a.Version, a.Arch)
	wantType := "exec"
	perm := os.FileMode(0o755)
	if a.BinType == "gadget" {
		dir = p.GadgetDir()
		name += ".so"
		wantType = "dyn"
		perm = 0o644
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return core.Binary{}, err
	}

	lower := strings.ToLower(a.Name)
	ext := ""
	switch {
	case strings.HasSuffix(lower, ".xz"):
		ext = ".xz"
	case strings.HasSuffix(lower, ".gz"):
		ext = ".gz"
	}
	tmp := filepath.Join(dir, name+ext)
	tf, err := os.Create(tmp)
	if err != nil {
		return core.Binary{}, err
	}
	cl := NewClient(s.Download)
	sum, err := Fetch(ctx, cl, cl.DownloadURL(a), tf)
	tf.Close()
	if err != nil {
		os.Remove(tmp)
		return core.Binary{}, err
	}

	final := tmp
	if ext != "" {
		if final, err = Decompress(tmp, ext); err != nil {
			return core.Binary{}, err
		}
	}
	if err := os.Chmod(final, perm); err != nil {
		return core.Binary{}, err
	}

	// 终验：ELF 头必须与声称的 arch/type 一致，不一致则清理报错
	info, err := core.InspectFile(final)
	if err != nil || info.Arch != a.Arch || info.Type != wantType {
		os.Remove(final)
		return core.Binary{}, fmt.Errorf("post-download verify failed: %+v err=%v (want %s/%s)", info, err, a.Arch, wantType)
	}

	st, err := os.Stat(final)
	if err != nil {
		return core.Binary{}, err
	}
	if final != filepath.Join(dir, name) {
		if err := os.Rename(final, filepath.Join(dir, name)); err != nil {
			return core.Binary{}, err
		}
	}
	b := core.Binary{
		File: name, Variant: variant, Version: a.Version, Arch: a.Arch,
		ELFType: info.Type, Source: a.URL, SHA256: sum,
		Size: st.Size(), AddedAt: time.Now(),
	}
	m.Upsert(a.BinType, b)
	return b, core.SaveManifest(p, *m)
}
