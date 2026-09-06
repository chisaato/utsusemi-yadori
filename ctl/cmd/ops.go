package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"utsusemi/ctl/internal/binmgr"
	"utsusemi/ctl/internal/core"
	"utsusemi/ctl/internal/dl"
	"utsusemi/ctl/internal/gadgetcfg"
	"utsusemi/ctl/internal/srv"
	"utsusemi/ctl/internal/web"
)

// opsImpl 实现 web.Ops；REST handler 与 api 子命令共用，保证 JSON 同构
type opsImpl struct{ P core.Paths }

func opsFor(p core.Paths) web.Ops { return &opsImpl{P: p} }

func (o *opsImpl) Status() (any, error) {
	m, err := binmgr.New(o.P)
	if err != nil {
		return nil, err
	}
	r, err := gadgetcfg.LoadRules(o.P)
	if err != nil {
		return nil, err
	}
	// settings 摘要：不含 web.token 之外的敏感面（token 本就设计为可展示给本模块用户）
	return map[string]any{
		"server":   srv.New(o.P).Status(),
		"servers":  m.List("server"),
		"gadgets":  m.List("gadget"),
		"rules":    r,
		"web":      m.S.Web,
		"settings": map[string]any{"server": m.S.Server, "gadget": m.S.Gadget, "download": m.S.Download},
	}, nil
}

func (o *opsImpl) ServerAction(action string) (any, error) {
	m := srv.New(o.P)
	switch action {
	case "start":
		return m.Start()
	case "stop":
		return m.Stop()
	case "restart":
		if _, err := m.Stop(); err != nil {
			return nil, err
		}
		return m.Start()
	}
	return nil, fmt.Errorf("unknown action %q", action)
}

func (o *opsImpl) ServerSet(payload []byte) (any, error) {
	s, err := core.LoadSettings(o.P)
	if err != nil {
		return nil, err
	}
	// 全量替换 server 段（契约 §4.2）
	if err := json.Unmarshal(payload, &s.Server); err != nil {
		return nil, fmt.Errorf("bad server settings payload: %w", err)
	}
	if err := core.SaveSettings(o.P, s); err != nil {
		return nil, err
	}
	return map[string]any{"saved": true}, nil
}

func (o *opsImpl) Apps() (any, error) { return gadgetcfg.ListApps(nil) }

func (o *opsImpl) GadgetRules() (any, error) { return gadgetcfg.LoadRules(o.P) }

func (o *opsImpl) GadgetSet(payload []byte) (any, error) {
	var r gadgetcfg.Rules
	if err := json.Unmarshal(payload, &r); err != nil {
		return nil, fmt.Errorf("bad rules payload: %w", err)
	}
	if err := r.Save(o.P); err != nil {
		return nil, err
	}
	out := map[string]any{"saved": true, "applied": 0}
	// 规则已保存；apply 失败不整体失败（如尚无激活 gadget），错误带给前端提示
	if err := gadgetcfg.Apply(o.P); err != nil {
		out["apply_error"] = err.Error()
	} else {
		n := 0
		for _, rule := range r.Rules {
			if rule.Enabled {
				n++
			}
		}
		out["applied"] = n
	}
	return out, nil
}

func (o *opsImpl) BinList() (any, error) {
	m, err := binmgr.New(o.P)
	if err != nil {
		return nil, err
	}
	return map[string]any{"servers": m.List("server"), "gadgets": m.List("gadget")}, nil
}

func (o *opsImpl) BinSources() (any, error) {
	m, err := binmgr.New(o.P)
	if err != nil {
		return nil, err
	}
	cl := dl.NewClient(m.S.Download)
	out := []map[string]any{}
	for _, v := range dl.Variants {
		vs, err := cl.ListVersions(context.Background(), v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", v, err)
		}
		out = append(out, map[string]any{"variant": v, "versions": vs})
	}
	return out, nil
}

// doDownload 解析版本→找资产→安装；upd 上报进度（同步模式传 no-op）
func (o *opsImpl) doDownload(ctx context.Context, m *binmgr.Manager, variant, binType, version string, upd func(phase, detail string) error) (core.Binary, error) {
	if binType == "" {
		binType = "server"
	}
	upd("resolve", variant)
	if version == "" {
		cl := dl.NewClient(m.S.Download)
		vs, err := cl.ListVersions(ctx, variant)
		if err != nil {
			return core.Binary{}, err
		}
		if len(vs) == 0 {
			return core.Binary{}, fmt.Errorf("no releases with assets for %s", variant)
		}
		version = vs[0]
	}
	upd("download", variant+" "+version)
	cl := dl.NewClient(m.S.Download)
	a, err := cl.FindAsset(ctx, variant, version, binType, core.DeviceArch())
	if err != nil {
		return core.Binary{}, err
	}
	return dl.Install(ctx, m.P, m.S, m.M, variant, a)
}

func (o *opsImpl) BinDownload(variant, binType, version string, async bool, tm *web.TaskManager) (any, error) {
	m, err := binmgr.New(o.P)
	if err != nil {
		return nil, err
	}
	if !async {
		noOp := func(string, string) error { return nil }
		return o.doDownload(context.Background(), m, variant, binType, version, noOp)
	}
	id := web.NewTaskID()
	tm.Start(id, func(upd func(phase, detail string) error) error {
		_, err := o.doDownload(context.Background(), m, variant, binType, version, upd)
		return err
	})
	return map[string]string{"task_id": id}, nil
}

func (o *opsImpl) BinImport(binType, path, version string, async bool, tm *web.TaskManager) (any, error) {
	m, err := binmgr.New(o.P)
	if err != nil {
		return nil, err
	}
	if !async {
		return m.Import(binType, path, version)
	}
	id := web.NewTaskID()
	tm.Start(id, func(upd func(phase, detail string) error) error {
		upd("install", path)
		_, err := m.Import(binType, path, version)
		return err
	})
	return map[string]string{"task_id": id}, nil
}

func (o *opsImpl) BinRemove(binType, file string) (any, error) {
	m, err := binmgr.New(o.P)
	if err != nil {
		return nil, err
	}
	if err := m.Remove(binType, file); err != nil {
		return nil, err
	}
	return map[string]string{"removed": file}, nil
}

func (o *opsImpl) BinUse(binType, file string) (any, error) {
	m, err := binmgr.New(o.P)
	if err != nil {
		return nil, err
	}
	if err := m.SetActive(binType, file); err != nil {
		return nil, err
	}
	return map[string]string{"active": file}, nil
}

func (o *opsImpl) BinCleanup() (any, error) {
	m, err := binmgr.New(o.P)
	if err != nil {
		return nil, err
	}
	removed, err := m.Cleanup()
	if err != nil {
		return nil, err
	}
	return map[string][]string{"removed": removed}, nil
}

var logNames = map[string]func(core.Paths) string{
	"ctl":    core.Paths.CtlLog,
	"server": core.Paths.ServerLog,
	"web":    core.Paths.WebLog,
}

func (o *opsImpl) Logs(name string, tail int) (any, error) {
	getter, ok := logNames[name]
	if !ok {
		return nil, fmt.Errorf("unknown log %q (ctl|server|web)", name)
	}
	if tail <= 0 {
		tail = 200
	}
	lines := []string{}
	raw, err := os.ReadFile(getter(o.P))
	if err == nil {
		for _, l := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
			lines = append(lines, l)
		}
		if len(lines) > tail {
			lines = lines[len(lines)-tail:]
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	return map[string]any{"name": name, "lines": lines}, nil
}

// webAlive 读 web.pid 并 kill 0 判活（cmdline 校验从宽：serve 进程即视为有效）
func webAlive(p core.Paths) bool {
	raw, err := os.ReadFile(p.WebPidFile())
	if err != nil {
		return false
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
	return pid > 0 && syscall.Kill(pid, 0) == nil
}

func (o *opsImpl) WebInfo() (any, error) {
	s, err := core.LoadSettings(o.P)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"running": webAlive(o.P),
		"enabled": s.Web.Enabled,
		"port":    s.Web.Port,
		"token":   s.Web.Token,
	}, nil
}
