# Utsusemi WebUI API 契约（Phase 3）

前端唯一事实来源。所有端点 REST 与 `utsusemi-ctl api` 子命令 1:1 同构。

## 1. 统一信封 Envelope

所有响应（REST 与 api 子命令 stdout）：

```json
{ "ok": true,  "data": { ... } }
{ "ok": false, "error": "人可读错误信息" }
```

- HTTP 层面始终返回 200（错误语义在信封内）；传输失败（进程退出码≠0 / 网络错误）按 `ok:false` 处理。
- `api` 子命令无视 `--json` 旗标，永远输出单行 JSON 到 stdout。

## 2. 双传输适配层（前端核心）

```
探测 window.kpu/ksu:
  存在 → KSU 管理器内置 WebUI 模式:
    ksu.exec("/data/adb/modules/utsusemi/bin/utsusemi-ctl api <sub> [--flags]", 300, cb)
    → stdout 末行 JSON.parse → Envelope
  不存在 → PC 远程模式 (Gin):
    fetch("/api/...", { headers: { Authorization: "Bearer <token>" } })
    → res.json() → Envelope
```

- token：入口 URL 携带 `?token=xxx`，前端立即存 `localStorage.utsusemi_token` 并清掉 URL 上的 query；此后统一走 header。
- ksu 模式命令拼接：参数一律 `--key value`；JSON payload 用单引号包裹（payload 内容不会含单引号）。

## 3. 端点映射表

| 功能 | REST | api 子命令 | 入参 | data 形状 |
|---|---|---|---|---|
| 总览 | `GET /api/status` | `api status` | — | Status（§4.1） |
| 启动 server | `POST /api/server/start` | `api server/start` | — | SrvStatus |
| 停止 | `POST /api/server/stop` | `api server/stop` | — | SrvStatus |
| 重启 | `POST /api/server/restart` | `api server/restart` | — | SrvStatus |
| 更新 server 设置 | `PUT /api/server` | `api server/set --payload '<json>'` | ServerSettings（§4.2） | `{"saved":true}` |
| 应用列表 | `GET /api/apps` | `api apps` | — | `[App]`（§4.3） |
| 读规则 | `GET /api/gadget` | `api gadget` | — | Rules（§4.4） |
| 写规则（自动 apply） | `PUT /api/gadget` | `api gadget/set --payload '<json>'` | Rules | `{"applied":<n>}` |
| 二进制清单 | `GET /api/bin` | `api bin/list` | — | `{"servers":[Binary],"gadgets":[Binary]}` |
| 可用版本 | `GET /api/bin/sources` | `api bin/sources` | — | `[{variant,versions:[str]}]` |
| 下载 | `POST /api/bin/download` | `api bin/download --variant --type --version` | `{"variant","type","version"?}` | REST: `{"task_id"}`；api 同步: Binary |
| 导入 | `POST /api/bin/import` (multipart: `file` + form `type`,`version`) | `api bin/import --type --file <path> --version` | — | REST: `{"task_id"}`；api 同步: Binary |
| 删除 | `DELETE /api/bin/:file?type=server\|gadget` | `api bin/remove --type --file` | — | `{"removed":"<file>"}` |
| 激活 | `POST /api/bin/use` | `api bin/use --type --file` | `{"type","file"}` | `{"active":"<file>"}` |
| 清理未激活 | `POST /api/bin/cleanup` | `api bin/cleanup` | — | `{"removed":["<file>"]}` |
| 任务进度 | `GET /api/tasks/:id` | `api tasks/<id>` | — | Task（§4.5） |
| 日志 | `GET /api/logs?name=ctl\|server\|web&tail=200` | `api logs --name --tail` | — | `{"name","lines":[str]}` |
| Web 服务信息 | `GET /api/web/info` | `api web/info` | — | WebInfo（§4.6） |
| 远程关停 | `POST /api/web/stop` | —（仅 REST） | — | `{"stopping":true}` |

## 4. 数据模型

### 4.1 Status（总览首屏一次拉全）

```json
{
  "server":  { "running": true, "pid": 12345, "binary": "official_17.2.14_arm64", "uptime_sec": 3600, "note": "" },
  "servers": [ Binary ],
  "gadgets": [ Binary ],
  "rules":   { "rules": [ Rule ] },
  "web":     { "enabled": false, "port": 23333, "token": "ab12cd34" },
  "settings": { "server": ServerSettings, "gadget": {"active":"official_17.2.14_arm64.so"}, "download": {"mirror":"","github_api":"https://api.github.com"} }
}
```

`server.note` 非空 = 异常原因（如 `no active server binary`）。

### 4.2 ServerSettings（PUT /api/server 的 body 全量替换）

```json
{ "autostart": true, "active": "official_17.2.14_arm64", "args": ["-l","127.0.0.1:27042"], "restart_on_crash": true }
```

### 4.3 App

```json
{ "package": "com.example.app", "system": false }
```

### 4.4 Rules / Rule（PUT /api/gadget 的 body 全量替换，写入后自动 apply 重建 gadget.json）

```json
{ "rules": [ { "app_name": "com.example.app", "enabled": true, "start_up_delay_ms": 0,
  "child_gating_enabled": false, "child_gating_mode": "freeze", "custom_gadget": "" } ] }
```

`custom_gadget` 空 = 用全局激活 gadget；`child_gating_mode` ∈ `freeze|wait|kill`。

### 4.5 Binary（manifest 全量元数据，列表页须完整展示）

```json
{ "file": "official_17.2.14_arm64", "variant": "official", "version": "17.2.14",
  "arch": "arm64", "elf_type": "exec", "source": "https://github.com/frida/frida/releases/tag/17.2.14",
  "sha256": "e3b0c44…", "size": 12345678, "added_at": "2026-09-07T12:00:00+08:00", "missing": false }
```

- `variant` ∈ `official|florida|undetected|custom`；`elf_type` ∈ `exec|dyn`（server=exec / gadget=dyn）；`arch` ∈ `arm64|arm|x86_64|x86|unknown`。
- `missing:true` = manifest 有记录但文件丢失（UI 须标红提示重新导入）。
- `file` 是不透明 key，**禁止解析文件名**还原任何元数据。

### 4.6 Task（下载/导入进度轮询）

```json
{ "id": "a1b2c3d4", "state": "running", "phase": "download", "detail": "official 17.2.14",
  "error": "", "created_at": "…", "updated_at": "…" }
```

`state` ∈ `running|done|error`；`phase` ∈ `download|decompress|install`（done 后无意义）。

### 4.7 WebInfo

```json
{ "running": true, "enabled": true, "port": 23333, "token": "ab12cd34" }
```

## 5. 错误处理约定

- 任何 `ok:false`：toast 展示 `error` 字符串即可，无错误码。
- 401（token 错误，REST 特有）：提示重新从入口 URL 进入。
- ksu.exec 退出码非 0 或 stdout 无 JSON：按传输错误提示。

## 6. 前端工程要求

- **栈**：Vue 3（`<script setup>` + TS）+ Vite + Naive UI；包管理器 Bun；不用 pinia/router 之外的状态库（router 用 vue-router，hash 模式——KSU WebView file-ish 场景最稳）。
- **构建**：`bun run build` → `dist/`；vite `base: './'`；产物会被拷贝到 `template/webroot/`（KSU WebUI）并 embed 进 Go 二进制（PC 模式），同一份。
- **主题**：Naive UI `darkTheme`/`null` 切换；默认跟随 `prefers-color-scheme`；手动切换持久化 `localStorage.utsusemi_theme`（`dark|light|auto`）。
- **布局**：移动优先（KSU WebUI 手机端 360-430px 是主场景），PC 宽屏自适应；底部 Tab 或侧栏由设计师定。
- **页面**（5 个）：
  1. 总览：server 运行态卡片 + 一键启停 + 激活版本 + 规则摘要；
  2. 应用注入：应用列表（搜索、系统应用开关过滤）→ 勾选启用、延时、child gating、custom gadget；
  3. 二进制管理：server/gadget 两组列表（manifest 全元数据：variant/version/arch/elf_type/sha256/size/added_at/missing 标记）、下载（选源/类型/版本）、导入（PC 模式 file 上传；ksu 模式隐藏或提示用 PC）、激活切换、删除、清理；下载中轮询 task 展示 phase；
  4. 远程协同：WebInfo 展示、开关（ksu 模式可 `api web/start`——见注）、二维码（当前 URL+token）、`adb forward tcp:<port> tcp:<port>` 指引、token 显示/说明；
  5. 日志：name 切换（ctl/server/web）+ tail 行数 + 刷新/自动刷新。
- 注：`api web/start [--port]` 存在（启动 Gin 后台服务），ksu 模式下远程协同页用它开关；REST 模式下该页只展示信息 + 关停按钮（`POST /api/web/stop`）。
- **安全提示**：PC 模式默认 0.0.0.0 监听，远程协同页须常驻「局域网内任何人都可访问」警示文案。
