package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Binary struct {
	File    string    `json:"file"`             // 目录内实际文件名 = key
	Variant string    `json:"variant"`          // official|florida|undetected|custom
	Version string    `json:"version"`          // 任意字符串，custom 允许 unknown
	Arch    string    `json:"arch"`             // arm64|arm|x86_64|x86
	ELFType string    `json:"elf_type"`         // exec|dyn
	Source  string    `json:"source,omitempty"` // 下载来源 URL
	SHA256  string    `json:"sha256"`
	Size    int64     `json:"size"`
	AddedAt time.Time `json:"added_at"`
	Missing bool      `json:"missing,omitempty"` // 对账标记：文件已丢失
}

type Manifest struct {
	Servers []Binary `json:"servers"`
	Gadgets []Binary `json:"gadgets"`
}

func LoadManifest(p Paths) (Manifest, error) {
	var m Manifest
	raw, err := os.ReadFile(p.Manifest())
	if os.IsNotExist(err) {
		return m, nil
	}
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(raw, &m)
}

func SaveManifest(p Paths, m Manifest) error {
	if err := p.Ensure(); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := p.Manifest() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p.Manifest())
}

func (m *Manifest) Binaries(binType string) *[]Binary {
	if binType == "gadget" {
		return &m.Gadgets
	}
	return &m.Servers
}

// Upsert 按 file key 合并写入：新条目的非零字段覆盖旧值，零值字段保留旧值（便于局部更新）
func (m *Manifest) Upsert(binType string, b Binary) {
	bs := m.Binaries(binType)
	for i := range *bs {
		if (*bs)[i].File == b.File {
			old := (*bs)[i]
			if b.Variant == "" {
				b.Variant = old.Variant
			}
			if b.Version == "" {
				b.Version = old.Version
			}
			if b.Arch == "" {
				b.Arch = old.Arch
			}
			if b.ELFType == "" {
				b.ELFType = old.ELFType
			}
			if b.Source == "" {
				b.Source = old.Source
			}
			if b.SHA256 == "" {
				b.SHA256 = old.SHA256
			}
			if b.Size == 0 {
				b.Size = old.Size
			}
			if b.AddedAt.IsZero() {
				b.AddedAt = old.AddedAt
			}
			b.Missing = old.Missing // 对账状态由 Reconcile 管理
			(*bs)[i] = b
			return
		}
	}
	*bs = append(*bs, b)
}

func (m *Manifest) Find(binType, file string) (Binary, bool) {
	for _, b := range *m.Binaries(binType) {
		if b.File == file {
			return b, true
		}
	}
	return Binary{}, false
}

// Remove 从清单移除条目（不动磁盘文件）
func (m *Manifest) Remove(binType, file string) bool {
	bs := m.Binaries(binType)
	for i := range *bs {
		if (*bs)[i].File == file {
			*bs = append((*bs)[:i], (*bs)[i+1:]...)
			return true
		}
	}
	return false
}

// Reconcile 目录↔manifest 对账：孤儿登记 custom/unknown，丢失打标
func Reconcile(p Paths, m *Manifest) []string {
	var notes []string
	for _, binType := range []string{"server", "gadget"} {
		dir := p.ServerDir()
		if binType == "gadget" {
			dir = p.GadgetDir()
		}
		// 丢失检测
		bs := m.Binaries(binType)
		for i := range *bs {
			_, err := os.Stat(filepath.Join(dir, (*bs)[i].File))
			miss := err != nil
			if miss != (*bs)[i].Missing {
				(*bs)[i].Missing = miss
				if miss {
					notes = append(notes, binType+"/"+(*bs)[i].File+" missing")
				}
			}
		}
		// 孤儿登记
		ents, _ := os.ReadDir(dir)
		for _, e := range ents {
			if e.IsDir() {
				continue
			}
			if _, ok := m.Find(binType, e.Name()); !ok {
				// 孤儿登记：能读出 ELF 头则尽量补全 arch/elf_type
				b := Binary{
					File: e.Name(), Variant: "custom", Version: "unknown",
					Arch: "unknown", ELFType: "unknown",
					AddedAt: time.Now(),
				}
				if info, err := InspectFile(filepath.Join(dir, e.Name())); err == nil {
					b.Arch, b.ELFType = info.Arch, info.Type
				}
				m.Upsert(binType, b)
				notes = append(notes, binType+"/"+e.Name()+" registered as orphan")
			}
		}
	}
	return notes
}
