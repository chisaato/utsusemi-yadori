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

// Fetch 流式下载到 w，返回 sha256；onBytes 上报 (done, total)
func Fetch(ctx context.Context, c *Client, url string, w io.Writer, expectedTotal int64, onBytes func(done, total int64)) (string, error) {
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

	// 进度计算：优先 resp.ContentLength，<=0 时用传入的 Asset.Size 兜底
	total := resp.ContentLength
	if total <= 0 {
		total = expectedTotal
	}

	h := sha256.New()
	mw := io.MultiWriter(w, h)
	buf := make([]byte, 32*1024)
	var done int64
	if onBytes != nil {
		onBytes(0, total)
	}
	for {
		nr, rerr := resp.Body.Read(buf)
		if nr > 0 {
			nw, werr := mw.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if werr == nil {
					werr = fmt.Errorf("invalid write result")
				}
			}
			done += int64(nw)
			if onBytes != nil {
				onBytes(done, total)
			}
			if werr != nil {
				return "", werr
			}
			if nr != nw {
				return "", io.ErrShortWrite
			}
		}
		if rerr != nil {
			if rerr == io.EOF {
				break
			}
			return "", rerr
		}
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
func Install(ctx context.Context, p core.Paths, s core.Settings, m *core.Manifest, variant string, a Asset, onBytes func(done, total int64)) (core.Binary, error) {
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
	sum, err := Fetch(ctx, cl, cl.DownloadURL(a), tf, a.Size, onBytes)
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
	// 注意：Android PIE 构建的 server 本身也是 dyn，允许 exec 或 dyn
	info, err := core.InspectFile(final)
	validType := (a.BinType == "gadget" && info.Type == "dyn") ||
		(a.BinType == "server" && (info.Type == "exec" || info.Type == "dyn"))
	if err != nil || info.Arch != a.Arch || !validType {
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
