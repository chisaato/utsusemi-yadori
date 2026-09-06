package gadgetcfg

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"utsusemi/ctl/internal/core"
)

// Rule 是用户层注入规则；CustomGadget 为空时使用 settings.gadget.active
type Rule struct {
	AppName            string `json:"app_name"`
	Enabled            bool   `json:"enabled"`
	StartUpDelayMS     int64  `json:"start_up_delay_ms"`
	ChildGatingEnabled bool   `json:"child_gating_enabled"`
	ChildGatingMode    string `json:"child_gating_mode"`
	CustomGadget       string `json:"custom_gadget,omitempty"`
}

type Rules struct {
	Rules []Rule `json:"rules"`
}

func LoadRules(p core.Paths) (Rules, error) {
	var r Rules
	raw, err := os.ReadFile(p.Rules())
	if os.IsNotExist(err) {
		return r, nil
	}
	if err != nil {
		return r, err
	}
	return r, json.Unmarshal(raw, &r)
}

func (r Rules) Save(p core.Paths) error {
	if err := p.Ensure(); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp := p.Rules() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p.Rules())
}

// —— ZygiskFrida 兼容 gadget.json 模型 ——

type Lib struct {
	Path string `json:"path"`
}

type ChildGating struct {
	Enabled           bool  `json:"enabled"`
	Mode              string `json:"mode"`
	InjectedLibraries []Lib `json:"injected_libraries"`
}

type Target struct {
	AppName           string      `json:"app_name"`
	Enabled           bool        `json:"enabled"`
	StartUpDelayMS    int64       `json:"start_up_delay_ms"`
	InjectedLibraries []Lib       `json:"injected_libraries"`
	ChildGating       ChildGating `json:"child_gating"`
}

type Config struct {
	Targets []Target `json:"targets"`
}

// Apply 由 rules+settings+manifest 生成发布区 gadget.json，并把 gadget 同步到发布区（原子写）
func Apply(p core.Paths) error {
	s, err := core.LoadSettings(p)
	if err != nil {
		return err
	}
	m, err := core.LoadManifest(p)
	if err != nil {
		return err
	}
	r, err := LoadRules(p)
	if err != nil {
		return err
	}
	if err := p.EnsureStage(); err != nil {
		return err
	}

	// resolve: manifest key → 控制区源文件 → 同步到发布区（sha 相同跳过）
	resolve := func(key string) (string, error) {
		if key == "" {
			key = s.Gadget.Active
		}
		if key == "" {
			return "", fmt.Errorf("no active gadget configured")
		}
		b, ok := m.Find("gadget", key)
		if !ok {
			return "", fmt.Errorf("gadget %q not in manifest", key)
		}
		if b.Missing {
			return "", fmt.Errorf("gadget %q file missing on disk", key)
		}
		src := filepath.Join(p.GadgetDir(), key)
		dst := filepath.Join(p.Stage, key)
		if err := syncIfChanged(src, dst); err != nil {
			return "", err
		}
		return dst, nil
	}

	cfg := Config{Targets: []Target{}}
	for _, rule := range r.Rules {
		if !rule.Enabled {
			continue
		}
		path, err := resolve(rule.CustomGadget)
		if err != nil {
			return fmt.Errorf("app %s: %w", rule.AppName, err)
		}
		cfg.Targets = append(cfg.Targets, Target{
			AppName:           rule.AppName,
			Enabled:           true,
			StartUpDelayMS:    rule.StartUpDelayMS,
			InjectedLibraries: []Lib{{Path: path}},
			ChildGating: ChildGating{
				Enabled: rule.ChildGatingEnabled,
				Mode:    rule.ChildGatingMode,
			},
		})
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := p.GadgetConfig() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p.GadgetConfig())
}

// —— 应用列表 ——

type App struct {
	Package string `json:"package"`
	System  bool   `json:"system"`
}

// ListApps 通过 pm 列出包（-3=第三方；全量−第三方=系统）；runner 可注入便于测试
func ListApps(run func(name string, args ...string) (string, error)) ([]App, error) {
	if run == nil {
		run = func(name string, args ...string) (string, error) {
			out, err := exec.Command(name, args...).Output()
			return string(out), err
		}
	}
	userOut, err := run("pm", "list", "packages", "-3")
	if err != nil {
		return nil, fmt.Errorf("pm list packages: %w", err)
	}
	allOut, err := run("pm", "list", "packages")
	if err != nil {
		return nil, fmt.Errorf("pm list packages: %w", err)
	}
	user := pkgSet(userOut)
	all := pkgSet(allOut)
	apps := make([]App, 0, len(all))
	for pkg := range all {
		apps = append(apps, App{Package: pkg, System: !user[pkg]})
	}
	sort.Slice(apps, func(i, j int) bool { return apps[i].Package < apps[j].Package })
	return apps, nil
}

func pkgSet(out string) map[string]bool {
	set := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if after, ok := strings.CutPrefix(line, "package:"); ok {
			set[strings.TrimSpace(after)] = true
		}
	}
	return set
}

// syncIfChanged: 源与目标 sha256 相同则跳过，否则复制（0644）；发布区不是事实来源，可随时重建
func syncIfChanged(src, dst string) error {
	srcSum, err := fileSum(src)
	if err != nil {
		return err
	}
	if dstSum, err := fileSum(dst); err == nil && dstSum == srcSum {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".sync"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err = out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

func fileSum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
