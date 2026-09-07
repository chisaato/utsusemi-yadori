package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

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

// throttleTracker 用于 bytes 节流：≥300ms 或 ≥256KB 才触发写操作
type throttleTracker struct {
	lastTime  time.Time
	lastBytes int64
}

func (t *throttleTracker) shouldUpdate(bytes int64, now time.Time) bool {
	if t.lastTime.IsZero() {
		t.lastTime = now
		t.lastBytes = bytes
		return true
	}
	if now.Sub(t.lastTime) >= 300*time.Millisecond || bytes-t.lastBytes >= 256*1024 || bytes < t.lastBytes {
		t.lastTime = now
		t.lastBytes = bytes
		return true
	}
	return false
}

// writeSentinelTask 原子写入哨兵文件
func writeSentinelTask(p core.Paths, task web.Task) error {
	task.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return err
	}
	tmp := p.DownloadTaskFile() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p.DownloadTaskFile())
}

// doDownload 解析版本→找资产→安装；updProgress 上报进度（包含 phase, detail, done, total）
func (o *opsImpl) doDownload(ctx context.Context, m *binmgr.Manager, variant, binType, version string, updProgress func(phase, detail string, done, total int64)) (core.Binary, error) {
	if binType == "" {
		binType = "server"
	}
	if updProgress != nil {
		updProgress("resolve", variant, 0, 0)
	}
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
	if updProgress != nil {
		updProgress("download", variant+" "+version, 0, 0)
	}
	cl := dl.NewClient(m.S.Download)
	a, err := cl.FindAsset(ctx, variant, version, binType, core.DeviceArch())
	if err != nil {
		return core.Binary{}, err
	}
	onBytes := func(done, total int64) {
		if updProgress != nil {
			updProgress("download", variant+" "+version, done, total)
		}
	}
	b, err := dl.Install(ctx, m.P, m.S, m.M, variant, a, onBytes)
	if err != nil {
		return core.Binary{}, err
	}
	if updProgress != nil {
		updProgress("install", b.File, b.Size, b.Size)
	}
	return b, nil
}

func (o *opsImpl) BinDownload(variant, binType, version string, async bool, tm *web.TaskManager) (any, error) {
	m, err := binmgr.New(o.P)
	if err != nil {
		return nil, err
	}
	if !async {
		// ksu 同步执行模式：写哨兵文件 + 节流上报
		task := web.Task{
			ID:         "current",
			State:      "running",
			Phase:      "resolve",
			Detail:     variant,
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
			BytesDone:  0,
			BytesTotal: 0,
		}
		_ = writeSentinelTask(o.P, task)
		tracker := &throttleTracker{}
		updProgress := func(phase, detail string, done, total int64) {
			task.Phase = phase
			task.Detail = detail
			task.BytesDone = done
			task.BytesTotal = total
			now := time.Now()
			// phase 变更或满足节流阈值时写盘
			if phase != task.Phase || tracker.shouldUpdate(done, now) {
				_ = writeSentinelTask(o.P, task)
			}
		}
		b, err := o.doDownload(context.Background(), m, variant, binType, version, updProgress)
		if err != nil {
			task.State = "error"
			task.Error = err.Error()
			_ = writeSentinelTask(o.P, task)
			return nil, err
		}
		task.State = "done"
		task.Phase = "install"
		task.Detail = b.File
		task.BytesDone = b.Size
		task.BytesTotal = b.Size
		_ = writeSentinelTask(o.P, task)
		return b, nil
	}

	id := web.NewTaskID()
	tm.Start(id, func(legacyUpd func(phase, detail string) error) error {
		tracker := &throttleTracker{}
		updProgress := func(phase, detail string, done, total int64) {
			now := time.Now()
			if phase == "resolve" || phase == "install" || tracker.shouldUpdate(done, now) {
				tm.UpdateProgress(id, phase, detail, done, total)
			}
		}
		_, err := o.doDownload(context.Background(), m, variant, binType, version, updProgress)
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
	// 实际端口以 web.port 文件为准（--port 0 自动分配场景 serve 回写）
	port := s.Web.Port
	if raw, err := os.ReadFile(o.P.WebPortFile()); err == nil {
		if v, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil && v > 0 {
			port = v
		}
	}
	return map[string]any{
		"running": webAlive(o.P),
		"enabled": s.Web.Enabled,
		"port":    port,
		"token":   s.Web.Token,
	}, nil
}

func validateToken(token string) error {
	trimmed := strings.TrimSpace(token)
	if len(trimmed) < 8 || len(trimmed) > 128 {
		return fmt.Errorf("token length must be 8-128 chars")
	}
	for i := 0; i < len(trimmed); i++ {
		b := trimmed[i]
		if b < 33 || b > 126 { // 不含空白与控制字符，全 ASCII 可打印
			return fmt.Errorf("token contains invalid char %q", b)
		}
	}
	return nil
}

func (o *opsImpl) WebToken(payload []byte) (any, error) {
	s, err := core.LoadSettings(o.P)
	if err != nil {
		return nil, err
	}
	var req struct {
		Token    *string `json:"token"`
		Generate bool    `json:"generate"`
	}
	if len(payload) > 0 && strings.TrimSpace(string(payload)) != "" && string(payload) != "{}" {
		if err := json.Unmarshal(payload, &req); err != nil {
			return nil, fmt.Errorf("invalid json payload: %w", err)
		}
	}

	targetToken := s.Web.Token
	changed := false
	if req.Generate {
		targetToken = core.GenToken()
		changed = true
	} else if req.Token != nil {
		trimmed := strings.TrimSpace(*req.Token)
		if err := validateToken(trimmed); err != nil {
			return nil, err
		}
		targetToken = trimmed
		changed = true
	}

	if changed {
		s.Web.Token = targetToken
		if err := core.SaveSettings(o.P, s); err != nil {
			return nil, err
		}
	}

	return map[string]any{
		"token":            targetToken,
		"restart_required": webAlive(o.P),
	}, nil
}

func (o *opsImpl) TaskCurrent() (any, error) {
	raw, err := os.ReadFile(o.P.DownloadTaskFile())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no current task")
		}
		return nil, err
	}
	var t web.Task
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, fmt.Errorf("malformed sentinel task: %w", err)
	}
	return t, nil
}
