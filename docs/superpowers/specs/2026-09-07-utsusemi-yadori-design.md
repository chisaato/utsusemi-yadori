# Utsusemi-Yadori (蝉宿) 设计文档

- 日期: 2026-09-07
- 状态: 待用户审核
- 模块 ID: `utsusemi`（module.prop 中的 id）
- 数据目录: `/data/adb/utsusemi`

## 1. 概述与目标

一个面向 Magisk / KernelSU / SukiSU / APatch 环境的一体化 Frida 管理模块：

1. **frida-server 生命周期管理**：开机自启、启停、状态探测、状态回写 module.prop。
2. **frida-gadget 按应用注入**：基于 Zygisk（源码源自 ZygiskFrida），读取配置对目标应用注入 Gadget。
3. **二进制零内置**：安装包不携带任何 Frida 组件；安装后在线下载（官方 / Florida / undetected-frida / 自定义源），或导入用户自编译二进制。
4. **双模 WebUI**：
   - 手机端：KSU 系管理器内置 WebUI（webroot + `kernelsu` JS API 直调 ctl）；
   - PC 端：按需启动的 Gin Web 服务，局域网/adb forward 远程协同调试。

### 非目标

- 不做 Riru 支持（仅 Zygisk；Magisk 需开启 Zygisk，KSU 系依赖 ZygiskNext/SukiSU 内建实现）。
- 不内置任何 Frida 二进制。
- 不做自定义 Recovery 安装兼容。

## 2. 系统架构

```text
┌─────────────────────────────────────────────────────────┐
│  手机端                                                   │
│  ┌──────────────┐   ksu.exec(".../utsusemi-ctl ...")    │
│  │ KSU WebUI    │──────────────┐                        │
│  │ (webroot/)   │              ▼                        │
│  └──────────────┘      ┌───────────────┐   exec/stop    │
│                        │ utsusemi-ctl  │──────────────► frida-server
│  ┌──────────────┐ REST │  (Go, Gin)    │  生成 gadget.json
│  │ PC 浏览器     │─────►│  按需启动模式  │──────────────► Zygisk so
│  │ (局域网/adb)  │      └───────┬───────┘        ▲ 读取
│  └──────────────┘              │                │
│                         下载/解压/清单管理         │
│                                ▼                │
│                    /data/adb/utsusemi/frida-bin  │
│                                ┌────────────────┘
│                                │ 读取 config
│                     ┌──────────┴─────────┐
│                     │ Zygisk 模块 (C++)   │
│                     │ postAppSpecialize  │──► dlopen gadget 到目标 App
│                     └────────────────────┘
└─────────────────────────────────────────────────────────┘
```

组件职责：

| 组件 | 技术 | 职责 |
|---|---|---|
| `utsusemi-ctl` | Go + Gin | 唯一控制面：server 管理、gadget 配置生成、二进制下载/导入/清单、REST API + 内嵌前端 |
| Zygisk 模块 | C++ (NDK) | 进程拦截与 Gadget 注入（含 child gating、延时注入） |
| WebUI 前端 | Vue 3 + Bun (Vite) | 一套代码双模运行（KSU WebView / PC 浏览器） |
| 模块模板 | shell | service.sh / action.sh / customize.sh / uninstall.sh / verify.sh |

## 3. 仓库结构

```text
utsusemi-yadori/
├── ctl/                        # Go 管理工具
│   ├── main.go
│   ├── cmd/                    # cobra 子命令: server/gadget/bin/web/api/version
│   ├── internal/
│   │   ├── core/               # 路径、架构探测、settings/manifest 读写
│   │   ├── frida/              # frida-server 进程管理、探活
│   │   ├── gadget/             # 注入配置模型 + gadget.json 生成
│   │   ├── downloader/         # GitHub Releases 多源下载、xz/gz 解压、校验
│   │   └── web/                # Gin server: REST API + embed 前端 + token 鉴权
│   ├── webui/dist/             # (构建产物) go:embed 的前端
│   └── go.mod
├── native/zygisk/              # Zygisk C++ 源码（已从 ZygiskFrida 迁入）
├── web/                        # Vue 前端源码
├── template/                   # Magisk/KSU 模块模板（module.prop、*.sh、webroot/）
└── scripts/build.sh            # 一键构建: bun build → go build → ndk → zip
```

## 4. 设备侧目录与数据模型

### 4.1 目录布局（扁平化，无软链接）

```text
/data/adb/utsusemi/
├── settings.json        # 全局设置（激活项指向文件名，非路径链接）
├── gadget.json          # Zygisk 注入配置（ctl 生成，原子写入）
├── manifest.json        # frida-bin 清单（所有二进制的元数据索引）
├── logs/                # ctl / frida-server 日志
└── frida-bin/
    ├── server/
    │   ├── official_17.2.14_arm64
    │   ├── florida_17.9.1_arm64
    │   └── <用户导入文件保留原名，如 frida-server-test>
    └── gadget/
        ├── official_17.2.14_arm64.so
        ├── florida_17.9.1_arm64.so
        └── <用户导入文件保留原名>
```

设计要点：

- **manifest.json 是唯一事实来源（Single Source of Truth），文件名只是不透明 key**。系统任何功能都不解析文件名；`{variant}_{version}_{arch}` 仅是 ctl 自己下载时的**命名建议**，用户导入的文件**保留原始文件名**，元数据全部记录在 manifest 中。用户改文件名/用任意名字导入都不会破坏系统——启动时按 settings 引用的 key 查 manifest 拿到实际路径即可。
- **导入时 ELF 自动探测（Go `debug/elf` 标准库）**，不信任任何用户输入：
  - `e_machine` → 自动判定 `arm64 / arm / x86_64 / x86`；
  - `e_type` → server 须为 `ET_EXEC`（可执行），gadget 须为 `ET_DYN`（共享库）；
  - 非 ELF、架构与设备不符、类型与 bin 类型不符 → 导入直接拒绝并给出原因；
  - 与设备架构不匹配的文件仍允许导入（多架构设备/备份场景），但列表面会标注"非本机架构"且不可激活。
- **无软链接**。激活与否完全由 `settings.json` 中的文件 key 决定，ctl 启动 server 时直接 `exec` 对应文件；生成 `gadget.json` 时把激活 gadget 展开为绝对路径。
- **两级扁平**：`frida-bin/{server,gadget}/` 之下不再按版本建目录，避免嵌套过深。
- **未知版本兼容**：`variant=custom` 时 `version` 允许任意字符串（默认 `unknown`，用户可填如 `mybuild-20260907`）。系统对 version 只做展示与去重，**不做语义解析**；文件 key 冲突（同名不同文件）时自动追加时间戳后缀。
- **一致性自愈**：ctl 启动与 `bin list` 时做目录↔manifest 对账——孤儿文件自动登记为 `custom/unknown`；manifest 中指向的文件丢失则标记失效并在 UI 提示重新导入，激活项失效时拒绝启动并报清晰错误。

### 4.2 settings.json

```json
{
  "server": {
    "autostart": true,
    "active": "official_17.2.14_arm64",
    "args": ["-l", "0.0.0.0:27042"],
    "restart_on_crash": true
  },
  "gadget": {
    "active": "official_17.2.14_arm64.so"
  },
  "web": {
    "enabled": false,
    "port": 23333,
    "token": "随机生成"
  }
}
```

### 4.3 manifest.json

```json
{
  "servers": [
    {
      "file": "official_17.2.14_arm64",
      "variant": "official",
      "version": "17.2.14",
      "arch": "arm64",
      "elf_type": "exec",
      "source": "https://github.com/frida/frida/releases/tag/17.2.14",
      "sha256": "...",
      "size": 12345678,
      "added_at": "2026-09-07T12:00:00+08:00"
    }
  ],
  "gadgets": [ { "file": "frida-gadget-test", "variant": "custom", "version": "unknown", "arch": "arm64", "elf_type": "dyn", "...": "..." } ]
}
```

注意：`file` 是目录内的实际文件名（key），元数据（variant/version/arch/elf_type）在导入/下载时一次性写入，之后**只信 manifest 不信文件名**。

### 4.4 gadget.json（Zygisk 层读取，兼容 ZygiskFrida 配置结构）

```json
{
  "targets": [
    {
      "app_name": "com.example.app",
      "enabled": true,
      "start_up_delay_ms": 0,
      "injected_libraries": [
        { "path": "/data/adb/utsusemi/frida-bin/gadget/official_17.2.14_arm64.so" }
      ],
      "child_gating": {
        "enabled": false,
        "mode": "freeze",
        "injected_libraries": []
      }
    }
  ]
}
```

与 ZygiskFrida 的 `config.json` 结构一致（配置路径在源码中改为 `/data/adb/utsusemi/gadget.json`），保持 native 层改动最小。

## 5. utsusemi-ctl 设计

### 5.1 CLI 命令（cobra）

```text
utsusemi-ctl server start|stop|status|restart     # frida-server 管理
utsusemi-ctl server set --active <file> ...       # 设置激活二进制/启动参数
utsusemi-ctl gadget list|apply|enable|disable     # 注入规则管理（apply 重写 gadget.json）
utsusemi-ctl gadget apps                          # 列出已安装应用（包名/标签/系统位）
utsusemi-ctl bin list                             # 列出 manifest
utsusemi-ctl bin sources                          # 列出可用源与最新版本
utsusemi-ctl bin download --variant official|florida|undetected --type server|gadget [--version x.y.z]
utsusemi-ctl bin import --type server|gadget --file <path> [--version unknown]
                                                   # --name 已移除: 保留原始文件名作 key
                                                   # arch/elf_type 由 ELF 头自动探测, 非法文件拒绝导入
utsusemi-ctl bin remove <file> / bin cleanup      # 删除未激活项
utsusemi-ctl web start|stop [--port]              # 按需启动/停止 Gin 服务
utsusemi-ctl api <subcommand>                     # KSU WebUI 桥接: 与 REST API 同构的 JSON 输出
utsusemi-ctl status                               # 汇总状态（WebUI 首屏一次拉全）
```

约定：所有子命令支持 `--json`，输出结构与 REST API 完全一致 —— 前端适配层只需实现两种 transport（ksu.exec / fetch），命令映射 1:1。

### 5.2 下载源

| variant | 仓库 | 产物格式 |
|---|---|---|
| official | `frida/frida` | `frida-server-<v>-android-<abi>.xz` / `frida-gadget-<v>-android-<abi>.so.xz` |
| florida | `Ylarod/Florida` | `florida-server-<v>-android-<abi>.gz` / `florida-gadget-<v>-android-<abi>.so.gz` |
| undetected | `ultrafunkamsterdam/undetected-frida` | `undetected-frida-server-<v>-android-<abi>.xz` 等（命名不完全统一，需按 release 逐项匹配） |

- 下载走 GitHub Releases API；支持配置镜像前缀（如 ghproxy）写入 settings。
- 自动匹配设备架构（`arm64/arm/x86_64/x86`），解压 xz/gz，chmod 0755，写入 manifest。
- "最新带产物的 release"逻辑：跳过无 assets 的 tag（Florida/undetected 常有无产物 tag）。

### 5.3 REST API（Gin，仅 web start 后可用）

```text
GET  /api/status                 # server 状态 + 激活项 + gadget 规则摘要
POST /api/server/{start|stop|restart}
PUT  /api/server                 # 更新设置
GET  /api/apps                   # 已安装应用列表
GET/PUT /api/gadget              # 注入规则读写（PUT 后原子重写 gadget.json）
GET  /api/bin | /api/bin/sources
POST /api/bin/download           # 异步任务，返回 task id
POST /api/bin/import             # multipart 上传自编译二进制
GET  /api/tasks/:id              # 下载/解压进度
DELETE /api/bin/:file
POST /api/web/stop               # 远程关闭自身（防遗留）
GET  /api/logs                   # 日志尾部
```

- 鉴权：启动时生成一次性 token（`?token=` 或 `Authorization`），WebUI 展示二维码/URL；`adb forward` 场景可用 `--token` 固定。
- Gin 仅在 `web start` 后运行，无常驻进程；默认绑定 0.0.0.0（局域网可及），界面显著提示安全风险。

### 5.4 frida-server 进程管理

- `server start`：读 settings → 校验激活文件存在/可执行 → 后台拉起（setsid + nohup 语义）→ 写 pid 文件 → 轮询探活（进程存活 + `-l` 端口监听）→ 回写 module.prop description。
- `service.sh`（late_start）：等 `sys.boot_completed` 后按 `autostart` 决定是否启动。
- 崩溃自动重启（可选）：由 ctl 以 watchdog 子命令守护，默认关闭。

## 6. WebUI 前端设计

- 技术栈：Vue 3 + Vite + Bun，UI 库倾向 Naive UI / Nuxt UI 按团队熟悉度定（实现阶段确认）。
- **一份产物两处消费**：
  1. 构建输出拷贝到 `template/webroot/`（KSU 系 WebUI 直接加载）；
  2. 同一产物 embed 进 Go 二进制（`go:embed webui/dist`），供 Gin 远程模式伺服。
- 传输适配层（`src/api/index.ts`）：
  - 探测 `window.ksu`（或 `kernelsu` npm 包可用）→ `ksu.exec("/data/adb/modules/utsusemi/bin/utsusemi-ctl api ...")`；
  - 否则 → `fetch('/api/...', { headers: { Authorization: token } })`。
- 页面：
  1. **总览**：server 运行状态、激活版本、注入规则数、一键启停；
  2. **应用注入**：应用列表（搜索/系统应用过滤）→ 配置延时、child gating、启停；
  3. **二进制管理**：三源版本对比、下载/切换激活、上传自编译、删除清理；
  4. **远程协同**：开关 Gin 服务、展示二维码与 `adb forward` 指引、token 管理；
  5. **日志**：ctl 与 frida-server 日志查看。

## 7. 模块模板与安装/升级流程

### 7.1 模板结构（template/ → zip 根）

```text
module.prop            # id=utsusemi, version, updateJson 预留
customize.sh           # 安装钩子（见下）
service.sh             # late_start: 等 boot 后调 ctl server start（autostart）
action.sh              # 管理器 Action 按钮: ctl server 切换启停
uninstall.sh           # 清理 /data/adb/utsusemi（提示先导出）
verify.sh              # ABI 检查（arm64/arm/x86_64/x86）
post-fs-data.sh        # （保留空实现或仅做日志目录准备）
webroot/               # 前端构建产物（KSU WebUI 入口）
bin/utsusemi-ctl       # Go 产物（多架构，打包时按目标 ABI 选择）
zygisk/arm64-v8a.so 等 # NDK 产物
META-INF/…             # 保留 ZygiskFrida 的 update-binary 机制或用管理器内置
```

### 7.2 customize.sh 关键步骤（覆盖安装防残留）

依据 KSU/Magisk 文档：customize.sh 在文件解压并赋默认权限/SELinux 后被 source。前端产物文件名带 contenthash，升级安装若不清理会累积残留，因此：

```sh
# webroot 必须由本次安装的 zip 重新提供，先清后拷：
rm -rf "$MODPATH/webroot"
unzip -o "$ZIPFILE" "webroot/*" -d "$MODPATH" >/dev/null
# KSU 会自动处理 webroot 权限与 secontext；不手动 set_perm webroot
set_perm_recursive "$MODPATH/bin" 0 0 0755 0755
set_perm "$MODPATH/zygisk" …  # 按 Zygisk 惯例 0755
```

- 数据目录 `/data/adb/utsusemi`（settings/manifest/frida-bin）**不在模块目录内**，升级模块不丢数据；首次安装自动创建并写入默认 settings。
- 兼容判定：`$KSU` / `$MAGISK_VER` 区分环境；Zygisk 可用性检测（`/data/adb/zygisk` 或 ZygiskNext 存在性）在 verify 阶段仅警告不阻断。

## 8. 构建流水线（scripts/build.sh）

1. `web/`：`bun install && bun run build` → 产物拷贝至 `template/webroot/` 与 `ctl/webui/dist/`（后者供 embed）；
2. `ctl/`：`CGO_ENABLED=0 go build`，按目标 ABI 交叉编译（android/arm64、android/arm、android/amd64、android/386）；
3. `native/zygisk/`：NDK（ndk-build 或迁移 CMake）产出 `zygisk/<abi>.so`；
4. 组装 `template/` + 产物 → `utsusemi-<version>.zip`（LF 行尾校验、可执行位校验）。

CI（GitHub Actions）后续在实现阶段补充：矩阵打包 + release 附 updateJson。

## 9. 测试策略

- **ctl 单元测试**：settings/manifest 读写、文件名解析（含 unknown 版本）、下载解压（本地 fixture 模拟 xz/gz）、gadget.json 原子写入。
- **CLI 集成**：`utsusemi-ctl --json` 各子命令在临时目录数据根下跑通（`--data-root` 注入便于测试）。
- **Zygisk**：沿用 ZygiskFrida 行为，真机验证注入目标进程；child gating 冒烟。
- **WebUI**：mock 两种 transport 的 e2e（本地 gin 起 api + 无 ksu 环境）。
- **安装升级**：同版本覆盖安装、跨版本升级（验证 webroot 清理、数据保留）。

## 10. 风险与开放问题

1. **SELinux**：zygisk so 注入 app 进程读 `/data/adb/utsusemi/frida-bin/` 的权限/上下文需真机确认（ZygiskFrida 用 `/data/local/tmp`；迁到 `/data/adb` 后需 sepolicy.rule 或沿用其 remapper 规避）。→ 实现首日真机验证，必要时加 `sepolicy.rule`。
2. **undetected-frida 产物命名不规则**（有无 `.so` 后缀不一致）：downloader 按正则 + 类型启发式匹配，失败时允许用户手填 URL。
3. **Gin 远程服务安全**：仅按需启动 + token；文档中警示局域网风险。
4. **module.prop description 动态回写**在不同管理器的缓存行为差异（部分管理器缓存描述）——可接受，作为已知限制记录。
