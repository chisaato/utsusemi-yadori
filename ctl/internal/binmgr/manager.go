package binmgr

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"utsusemi/ctl/internal/core"
)

type Manager struct {
	P core.Paths
	S core.Settings
	M *core.Manifest
}

func New(p core.Paths) (*Manager, error) {
	if err := p.Ensure(); err != nil {
		return nil, err
	}
	s, err := core.LoadSettings(p)
	if err != nil {
		return nil, err
	}
	m, err := core.LoadManifest(p)
	if err != nil {
		return nil, err
	}
	core.Reconcile(p, &m)
	return &Manager{P: p, S: s, M: &m}, nil
}

func (m *Manager) Save() error {
	if err := core.SaveSettings(m.P, m.S); err != nil {
		return err
	}
	return core.SaveManifest(m.P, *m.M)
}

func (m *Manager) dir(binType string) string {
	if binType == "gadget" {
		return m.P.GadgetDir()
	}
	return m.P.ServerDir()
}

// Import 校验 ELF → 复制（保留原文件名，冲突加时间戳）→ 登记 manifest
func (m *Manager) Import(binType, srcPath, version string) (core.Binary, error) {
	if binType != "server" && binType != "gadget" {
		return core.Binary{}, fmt.Errorf("invalid type %q, want server|gadget", binType)
	}
	info, err := core.InspectFile(srcPath)
	if err != nil {
		return core.Binary{}, fmt.Errorf("inspect %s: %w", srcPath, err)
	}
	// server 允许 exec 或 dyn（Android NDK 编译的 PIE 二进制均为 ET_DYN）
	// gadget 必须为 dyn（共享库）
	if binType == "gadget" && info.Type != "dyn" {
		return core.Binary{}, fmt.Errorf("%s: want ELF dyn, got %s", srcPath, info.Type)
	}
	if binType == "server" && info.Type != "exec" && info.Type != "dyn" {
		return core.Binary{}, fmt.Errorf("%s: want ELF exec/dyn, got %s", srcPath, info.Type)
	}
	if version == "" {
		version = "unknown"
	}

	base := filepath.Base(srcPath)
	dest := filepath.Join(m.dir(binType), base)
	if _, err := os.Stat(dest); err == nil {
		// 同名不同文件：追加时间戳
		ext := filepath.Ext(base)
		stem := strings.TrimSuffix(base, ext)
		base = fmt.Sprintf("%s_%s%s", stem, time.Now().Format("20060102150405"), ext)
		dest = filepath.Join(m.dir(binType), base)
	}
	if err := copyFile(srcPath, dest, binType == "server"); err != nil {
		return core.Binary{}, err
	}
	sum, size, err := fileDigest(dest)
	if err != nil {
		return core.Binary{}, err
	}
	b := core.Binary{
		File: filepath.Base(dest), Variant: "custom", Version: version,
		Arch: info.Arch, ELFType: info.Type,
		SHA256: sum, Size: size, AddedAt: time.Now(),
	}
	m.M.Upsert(binType, b)
	return b, m.Save()
}

func (m *Manager) List(binType string) []core.Binary { return *m.M.Binaries(binType) }

// activeKey 返回当前激活 file key
func activeKey(s core.Settings, binType string) string {
	if binType == "gadget" {
		return s.Gadget.Active
	}
	return s.Server.Active
}

func (m *Manager) Remove(binType, file string) error {
	if binType != "server" && binType != "gadget" {
		return fmt.Errorf("invalid type %q", binType)
	}
	if activeKey(m.S, binType) == file {
		return fmt.Errorf("%s is active, refuse removal", file)
	}
	if _, ok := m.M.Find(binType, file); !ok {
		return fmt.Errorf("no such %s: %s", binType, file)
	}
	if err := os.Remove(filepath.Join(m.dir(binType), file)); err != nil && !os.IsNotExist(err) {
		return err
	}
	m.M.Remove(binType, file)
	return m.Save()
}

// Cleanup 删除两类中所有非激活项
func (m *Manager) Cleanup() ([]string, error) {
	var removed []string
	for _, binType := range []string{"server", "gadget"} {
		for _, b := range append([]core.Binary(nil), *m.M.Binaries(binType)...) {
			if b.File == activeKey(m.S, binType) {
				continue
			}
			if err := m.Remove(binType, b.File); err == nil {
				removed = append(removed, binType+"/"+b.File)
			}
		}
	}
	return removed, m.Save()
}

// SetActive 校验存在性/缺失/架构后写 settings
func (m *Manager) SetActive(binType, file string) error {
	if binType != "server" && binType != "gadget" {
		return fmt.Errorf("invalid type %q", binType)
	}
	b, ok := m.M.Find(binType, file)
	if !ok {
		return fmt.Errorf("no such %s: %s", binType, file)
	}
	if b.Missing {
		return fmt.Errorf("%s file missing on disk", file)
	}
	if b.Arch != core.DeviceArch() {
		return fmt.Errorf("arch mismatch: bin=%s device=%s", b.Arch, core.DeviceArch())
	}
	if binType == "gadget" {
		m.S.Gadget.Active = file
	} else {
		m.S.Server.Active = file
	}
	return m.Save()
}

func copyFile(src, dst string, exec bool) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	mode := os.FileMode(0o644)
	if exec {
		mode = 0o755
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func fileDigest(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
