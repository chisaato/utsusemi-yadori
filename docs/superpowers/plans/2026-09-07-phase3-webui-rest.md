# Phase 3: WebUI 与 REST API 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 Gin REST API + `api` CLI 桥 + `web start/stop` 按需服务 + go:embed，并交付 Vue3+Naive UI 双模前端（KSU WebUI / PC 远程）。

**Architecture:** `internal/web` 提供 Gin server、内存任务管理器与 token 中间件，业务动作经 `web.Ops` 接口注入；`cmd/ops.go` 实现该接口聚合现有 internal 包，CLI `api` 子命令与 REST handler 共用同一 ops 实现，保证 JSON 同构。前端一份产物两处消费（`template/webroot/` + `go:embed webui/dist`），传输适配层探测 `window.ksu` 选择 exec 或 fetch。

**Tech Stack:** Go 1.26 + gin-gonic/gin（新依赖）；Vue 3 + Vite + Bun + Naive UI（前端，@designer lane）。

**Spec:** `docs/superpowers/specs/2026-09-07-utsusemi-yadori-design.md` §5.1（api 子命令）、§5.3（REST）、§6（前端）。

## Global Constraints

- 所有 JSON 输出走统一信封 `{"ok":bool,"data":...,"error":"..."}`（root.go Envelope）
- `api` 子命令无视 `--json` 旗标，永远输出 JSON 信封（它就是给程序调用的）
- REST 与 `api` 子命令命令映射 1:1，唯一例外：download 在 REST 是异步任务（返回 task id + 轮询），在 `api` 子命令是同步执行（ksu.exec 阻塞语义）
- token 鉴权：`?token=` 或 `Authorization: Bearer <token>`；静态前端资源不鉴权（token 由入口 URL query 携带，前端存 localStorage 后用 header）
- `web` 服务只经 `web start` 显式启动，boot 不自启（spec 远程调试按需原则）
- 前端产物构建后同时拷贝 `template/webroot/` 与 `ctl/webui/dist/`
- CGO_ENABLED=0；Go 依赖新增仅 gin
- conventional commits；TDD 红→绿→commit
- 分支：`feat/phase3-webui`

---

### Task 1: internal/web 任务管理器

**Files:**
- Create: `ctl/internal/web/tasks.go`
- Test: `ctl/internal/web/tasks_test.go`

**Interfaces:**
- Consumes: 无（纯内存）
- Produces: `type Task struct{ID,State,Phase,Detail,Error string; CreatedAt,UpdatedAt time.Time}`；`type TaskManager struct`；方法 `NewTaskManager() *TaskManager`、`Start(id string, fn func(upd func(phase, detail string) error) error)`、`Get(id string) (Task, bool)`。State ∈ `running|done|error`。

- [ ] **Step 1: 写失败测试**

```go
package web

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestTaskLifecycle(t *testing.T) {
	tm := NewTaskManager()
	var wg sync.WaitGroup
	wg.Add(1)
	tm.Start("t1", func(upd func(phase, detail string) error) error {
		defer wg.Done()
		if err := upd("download", "official 17.2.14"); err != nil {
			return err
		}
		time.Sleep(50 * time.Millisecond)
		return nil
	})
	wg.Wait()
	time.Sleep(10 * time.Millisecond) // 等 Start 收尾写终态
	task, ok := tm.Get("t1")
	if !ok || task.State != "done" {
		t.Fatalf("want done, got %+v ok=%v", task, ok)
	}
}

func TestTaskError(t *testing.T) {
	tm := NewTaskManager()
	var wg sync.WaitGroup
	wg.Add(1)
	tm.Start("t2", func(upd func(phase, detail string) error) error {
		defer wg.Done()
		return errors.New("boom")
	})
	wg.Wait()
	time.Sleep(10 * time.Millisecond)
	task, _ := tm.Get("t2")
	if task.State != "error" || task.Error != "boom" {
		t.Fatalf("want error/boom, got %+v", task)
	}
}

func TestTaskProgressVisibleWhileRunning(t *testing.T) {
	tm := NewTaskManager()
	block := make(chan struct{})
	started := make(chan struct{})
	tm.Start("t3", func(upd func(phase, detail string) error) error {
		close(started)
		_ = upd("decompress", "xz")
		<-block
		return nil
	})
	<-started
	task, _ := tm.Get("t3")
	if task.State != "running" || task.Phase != "decompress" {
		t.Fatalf("want running/decompress, got %+v", task)
	}
	close(block)
	time.Sleep(10 * time.Millisecond)
	task, _ = tm.Get("t3")
	if task.State != "done" {
		t.Fatalf("want done, got %+v", task)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**（包不存在）
- [ ] **Step 3: 实现 tasks.go**

```go
package web

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// Task 是异步操作（下载/导入）的状态快照
type Task struct {
	ID        string    `json:"id"`
	State     string    `json:"state"`           // running | done | error
	Phase     string    `json:"phase,omitempty"` // download | decompress | install
	Detail    string    `json:"detail,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type TaskManager struct {
	mu    sync.RWMutex
	tasks map[string]*Task
}

func NewTaskManager() *TaskManager {
	return &TaskManager{tasks: map[string]*Task{}}
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Start 注册并异步执行 fn；upd 更新进度（phase/detail），fn 返回错误即终态 error
func (m *TaskManager) Start(id string, fn func(upd func(phase, detail string) error) error) {
	if id == "" {
		id = newID()
	}
	m.mu.Lock()
	m.tasks[id] = &Task{ID: id, State: "running", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	m.mu.Unlock()
	go func() {
		upd := func(phase, detail string) error {
			m.mu.Lock()
			defer m.mu.Unlock()
			t := m.tasks[id]
			if t == nil {
				return nil
			}
			t.Phase, t.Detail, t.UpdatedAt = phase, detail, time.Now()
			return nil
		}
		err := fn(upd)
		m.mu.Lock()
		defer m.mu.Unlock()
		t := m.tasks[id]
		if t == nil {
			return
		}
		t.UpdatedAt = time.Now()
		if err != nil {
			t.State, t.Error = "error", err.Error()
		} else {
			t.State = "done"
		}
	}()
}

func (m *TaskManager) Get(id string) (Task, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tasks[id]
	if !ok {
		return Task{}, false
	}
	return *t, true
}
```

- [ ] **Step 4: 跑测试确认通过**
- [ ] **Step 5: Commit** `feat(web): in-memory async task manager`

---

### Task 2: web.Ops 接口 + cmd/ops.go 实现 + api 子命令

**Files:**
- Create: `ctl/internal/web/ops.go`（仅接口定义，无实现）
- Create: `ctl/cmd/ops.go`
- Create: `ctl/cmd/api.go`
- Test: `ctl/cmd/api_test.go`

**Interfaces:**
- Consumes: 现有 binmgr/dl/srv/gadgetcfg/core 包 API（见各 cmd/*.go 现有用法）
- Produces: `web.Ops` 接口（方法签名见 ops.go 代码块）；`cmd.newAPICmd()`；`cmd.opsFor(p core.Paths) web.Ops`

**设计说明:** REST handler 与 CLI `api` 子命令共用 cmd 包内 `opsFor()` 返回的同一实现，杜绝三处逻辑漂移。Ops 全部返回 `(any, error)`，由调用方（Gin handler / cobra RunE）包 Envelope。

- [ ] **Step 1: 定义接口 `ctl/internal/web/ops.go`**

```go
package web

// Ops 是 REST 与 CLI api 子命令共用的业务动作集（实现于 cmd/ops.go）
// 所有方法返回可直接放入 Envelope.Data 的值
type Ops interface {
	Status() (any, error)
	ServerAction(action string) (any, error) // start | stop | restart
	ServerSet(payload []byte) (any, error)   // PUT body: settings.server 段
	Apps() (any, error)
	GadgetRules() (any, error)
	GadgetSet(payload []byte) (any, error) // PUT body: {"rules":[...]} 全量替换后 apply
	BinList() (any, error)
	BinSources() (any, error)
	BinDownload(variant, binType, version string, async bool, tm *TaskManager) (any, error)
	BinImport(binType, path string, version string, async bool, tm *TaskManager) (any, error)
	BinRemove(file string) (any, error)
	BinUse(binType, file string) (any, error)
	BinCleanup() (any, error)
	Logs(name string, tail int) (any, error)
	WebInfo() (any, error) // 端口/token/运行态，供前端远程协同页
}
```

- [ ] **Step 2: 实现 `ctl/cmd/ops.go`**：逐方法搬运现有 cmd/*.go RunE 内逻辑（status.go 的聚合、server.go 的启停、gadget.go 的规则、bin.go 的管理），搬运后原 RunE 改调 ops 方法（消除重复）。BinDownload/BinImport 支持 async=false 同步路径；async=true 时注册 TaskManager 任务立即返回 `map{"task_id": id}`。
- [ ] **Step 3: 实现 `ctl/cmd/api.go`**：`api <subcommand>`，子命令集 `status|server/start|server/stop|server/restart|server/set|apps|gadget|gadget/set|bin/list|bin/sources|bin/download|bin/import|bin/remove|bin/use|bin/cleanup|tasks/<id>|logs|web/info`。`--payload` 旗标（server/set、gadget/set 传 JSON 字符串）。全部输出 Envelope（强制 JSON，不用 asJSON）。
- [ ] **Step 4: 写测试 `ctl/cmd/api_test.go`**

```go
func TestAPISubcommandsJSON(t *testing.T) {
	// 与 cli_test.go 相同的 tempdir data-root 模式
	// 断言: api status 输出可解析为 Envelope 且 ok=true、data 含 server/servers/gadgets/rules/web 键
	// 断言: api gadget/set --payload '{"rules":[]}' → ok=true
	// 断言: api bin/list → data.servers 数组
	// 断言: api server/start（无激活二进制）→ ok=false 且 error 非空（不 panic）
}
```

- [ ] **Step 5: 红→绿循环后 Commit** `feat(api): isomorphic api subcommand backed by shared ops layer`

---

### Task 3: Gin server + token 中间件 + REST handlers

**Files:**
- Create: `ctl/internal/web/server.go`
- Test: `ctl/internal/web/server_test.go`
- Modify: `ctl/go.mod`（gin 依赖）

**Interfaces:**
- Consumes: Task 1 TaskManager、Task 2 Ops
- Produces: `type Server struct`；`NewServer(ops Ops, tm *TaskManager, token string, static fs.FS) *Server`；`(s *Server) Handler() http.Handler`；`(s *Server) Run(addr string) error`

- [ ] **Step 1: 写失败测试**（httptest 走全端点矩阵）

```go
func TestRESTMatrix(t *testing.T) {
	srv := NewServer(mockOps{}, NewTaskManager(), "tok", nil) // mockOps 实现 Ops 全方法
	h := srv.Handler()
	// 无 token → 401（/api/status）
	// ?token=tok → 200 且 {"ok":true}
	// Authorization: Bearer tok → 200
	// POST /api/server/start → mock 收到 action
	// POST /api/bin/download {"variant":"official","type":"server"} → {"ok":true,"data":{"task_id":"..."}} 且 GET /api/tasks/:id 可查
	// DELETE /api/bin/xxx → 200
	// GET /api/logs?name=ctl&tail=10 → 200
	// POST /api/web/stop → 200 且返回 special header "X-Utsusemi-Stop"（供 serve 循环优雅退出）
}
```

- [ ] **Step 2: 实现 server.go**：gin.SetMode(Release)；`/api` 路由组挂 token 中间件；handler 统一 `c.JSON(200, Envelope{...})`，ops 错误映射 `{"ok":false,"error":...}`（仍 200，信封语义）；multipart import：`c.FormFile("file")` 存 tmp 后调 `ops.BinImport`；`/api/web/stop` 触发注入的 shutdown 回调（`OnStop func()` 字段，serve 循环用它退出进程）；static fs.FS 非空时 `NoRoute` 伺服 `fs.Sub`，index.html 兜底。
- [ ] **Step 3: 红→绿**
- [ ] **Step 4: Commit** `feat(web): gin REST server with token auth and task endpoints`

---

### Task 4: web start/stop/status 命令 + web serve 内部子命令

**Files:**
- Create: `ctl/cmd/web.go`
- Test: `ctl/cmd/web_test.go`

**Interfaces:**
- Consumes: Task 3 Server；core.Settings.Web（Enabled/Port/Token）
- Produces: `web start [--port] [--token]`（后台 fork `utsusemi-ctl web serve`，写 logs/web.pid，token 空则生成随机写回 settings，输出 URL `http://<device-ip>:<port>/?token=...`）；`web stop`（pid 终止，enabled=false）；`web status`；内部 `web serve --port`（同步跑 Gin，日志 logs/web.log）

- [ ] **Step 1: 写失败测试**

```go
func TestWebLifecycle(t *testing.T) {
	// tempdir data-root；web status → data.running=false
	// web start --port 0 由 OS 分配（serve 写实际端口回 pid 文件旁的 web.port 文件）→ 轮询 GET /api/status 200
	// web status → data.running=true 且含 port/token
	// web stop → data.running=false，进程退出
}
```

（`--port 0` + 端口回写文件方案让集成测试无需固定端口竞态。）

- [ ] **Step 2: 实现 web.go**：start 复用 srv 包模式（setsid 后台子进程 + pidfile + 就绪轮询探测 HTTP）；serve 里 gin 跑前把实际 listener 端口写 `logs/web.port`；start 维护 settings.web.enabled/port/token。
- [ ] **Step 3: 红→绿**
- [ ] **Step 4: Commit** `feat(web): on-demand web service lifecycle commands`

---

### Task 5: go:embed 前端 + 静态伺服接线

**Files:**
- Create: `ctl/webui/dist/index.html`（占位，Task 7 被真产物覆盖）
- Create: `ctl/webui.go`
- Test: `ctl/cmd/web_test.go` 追加（GET / 返回 index.html 内容）

**Interfaces:**
- Consumes: Task 3 static 参数
- Produces: `//go:embed all:webui/dist`；`webui.FS()` 返回 embed FS

- [ ] **Step 1:** 占位 index.html（一行 `<h1>utsusemi</h1>`）+ webui.go embed
- [ ] **Step 2:** `NewServer(..., webui.FS())` 接线；测试 GET / 与 GET /assets 不存在时回落 index.html（SPA 兜底仅对非 /api 路径）
- [ ] **Step 3: 红→绿 → Commit** `feat(web): embed webui dist with SPA fallback`

---

### Task 6: 前端契约文档 + @designer lane（web/ 目录）

**Files:**
- Create: `docs/api-contract.md`（@designer 的唯一事实输入）
- Create: `web/`（Vue3+Vite+Bun+Naive UI 项目，@designer 全权实现）
- 规则：**web/ 目录写权限归 @designer lane**；orchestrator 只读

**契约要点（写入 docs/api-contract.md，需含真实 JSON 样例）:**
- Envelope 结构 + 各端点表（REST 路径 ↔ `utsusemi-ctl api <sub>` ↔ 方法/参数）
- 数据模型 JSON 形状：manifest entry（file/variant/version/arch/elf_type/source/sha256/size/added_at/missing）、rules（targets: app_name/enabled/start_up_delay_ms/child_gating）、status 聚合、task、logs
- 传输层：`window.ksu` 存在 → `ksu.exec("/data/adb/modules/utsusemi/bin/utsusemi-ctl api <sub> --payload '<json>'")`（timeout 给 300s；stdout 最后一行 JSON 解析）；否则 fetch `/api/...` + `Authorization: Bearer <token>`（token 从入口 URL query 捕获存 localStorage）；download 特判：fetch 模式轮询 `/api/tasks/:id`，ksu 模式同步等结果
- UI 要求：Naive UI；暗/亮自适应（prefers-color-scheme + 手动切换 + localStorage 持久化）；移动优先（KSU WebUI 手机是主场景）+ PC 响应式；五页面：总览/应用注入/二进制管理（manifest 全元数据表格）/远程协同（二维码+adb forward 指引+token）/日志
- 构建：`bun run build` → outDir `dist`，`base: './'`；产物被拷贝到 `template/webroot/` 与 `ctl/webui/dist/`

- [ ] **Step 1:** 写 docs/api-contract.md（从 Go struct 反推 + 现有测试 fixture 生成真实样例 JSON）
- [ ] **Step 2:** 委派 @designer 后台实现 web/（附契约文档路径 + 项目绝对路径 + 验证方式 `bun run build`）
- [ ] **Step 3:** designer 交付后验收：build 通过、产物结构合理、无 ksu 环境下本地 `web start` 冒烟页面可交互

---

### Task 7: 构建流水线集成

**Files:**
- Modify: `scripts/build.sh`（step 1: bun install/build → 拷贝 webroot + webui/dist）
- Modify: `scripts/assert-zip.sh`（webroot 非占位校验：index.html 引用 assets/ 带 hash 文件）

**Interfaces:**
- Consumes: Task 5 embed、Task 6 前端产物
- Produces: `dist/utsusemi-<ver>.zip` 含真前端

- [ ] **Step 1:** build.sh 加 `build_web()`（bun 可用检测，缺失则报错退出——前端已是必需品）
- [ ] **Step 2:** assert-zip.sh：webroot/index.html 存在且不含 `@`，webroot/assets 非空
- [ ] **Step 3:** 跑 `scripts/build.sh v0.2.0` + assert 通过 → Commit `build: bundle webui into module zip`

---

### Task 8: 收尾验证 + README + 合并

- [ ] `go test ./...` 全绿 + `go vet ./...`
- [ ] README：WebUI 使用（KSU 管理器打开 / PC `web start` + token + adb forward）、API 简表、开发（web/ 构建）
- [ ] finishing-a-development-branch → fast-forward 合并 main

## Self-Review 结论

- Spec 覆盖：§5.1 api 子命令（Task 2）、§5.3 REST 全端点（Task 3，含 spec 外补充的 bin/use、bin/cleanup——「命令映射 1:1」原则的必然推论）、§6 前端全部要求（Task 6）、§8 构建集成（Task 7）✓
- 补充决策（spec 未明说，已在此定死）：import REST 也走异步任务（与 download 统一轮询模式）；静态资源不鉴权；boot 不自启 web；SPA fallback 仅非 /api 路径
- 类型一致性：Ops 方法名在 Task 2/3 一致；TaskManager API 在 Task 1/3 一致 ✓
