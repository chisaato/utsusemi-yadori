# Utsusemi-Yadori Phase 2: Zygisk 适配 + 模块模板 + 打包 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 Zygisk native 层读取 `/data/local/tmp/utsusemi/gadget.json` 并注入 gadget；ctl 增加发布区同步与 `boot` 命令；产出完整可刷入的 Magisk/KSU 模块 zip（含 sha256 校验安装）。

**Architecture:** 三条线：① Go 侧增加 staging 发布区（`Paths.Stage`）与 `boot` 子命令；② native 侧剥离 Riru、vendor dobby 源码、配置路径改为 staging、Android.mk 换 CMake；③ 模板与打包（SKIPUNZIP=1 风格安装脚本 + 按设备 ABI 选装 + webroot 清理重拷 + build.sh 一键出 zip）。

**Tech Stack:** Go 1.26（既有 ctl）、NDK r28+（本机 `/data/.build-tools-cache/Android/ndk/28.2.13676358`）、CMake（dobby 为 CMake 项目）、bash 打包脚本。

**Spec:** `docs/superpowers/specs/2026-09-07-utsusemi-yadori-design.md`（本计划实现 §4.1 发布区、§7 模块模板与安装、§8 构建流水线中 native/打包部分；§7.2 webroot 清理逻辑在此落地；Gin/前端仍属 Phase 3）

## Global Constraints

- 沿用 Phase 1 全部约束（manifest 唯一事实来源、原子写、JSON 信封、conventional commits）。
- zygisk 源码改动最小化：只改配置路径与构建系统，注入逻辑不动。
- staging 默认路径 `/data/local/tmp/utsusemi`，必须经 `Paths` 注入以便测试（不硬编码在逻辑中）。
- 模块 zip 内脚本一律 LF 行尾；可执行文件从 zip 恢复权限后仍显式 `set_perm`。
- 所有 zip 内文件附 `.sha256sum`，`customize.sh` 经 `verify.sh` 的 `extract` 校验后落地。
- 不引入 gradle/prefab；dobby 以源码目录 `native/dobby` 引入（保留其 LICENSE）。

---

### Task 1: Paths.Stage 发布区 + gadgetcfg 发布逻辑

**Files:**
- Modify: `ctl/internal/core/paths.go`
- Modify: `ctl/internal/core/paths_test.go`
- Modify: `ctl/internal/gadgetcfg/config.go`
- Modify: `ctl/internal/gadgetcfg/config_test.go`
- Modify: `ctl/cmd/root.go`（`--stage` 持久旗标）

**Interfaces:**
- Consumes: Phase 1 全部
- Produces:
  - `Paths` 增加字段 `Stage string`；`New(root)` 默认 `Stage=/data/local/tmp/utsusemi`；新增 `NewStage(root, stage string) Paths`
  - `Paths.GadgetConfig()` 改为返回 `filepath.Join(p.Stage, "gadget.json")`
  - gadgetcfg `Apply(p)` 行为变更：解析 gadget key 后**复制到 staging**（`p.Stage/<file>`，sha256 相同则跳过），`injected_libraries.path` 指向 staging 文件；staging 目录自动创建（0755，文件 0644）
  - CLI：`--stage` 持久旗标覆盖默认 staging

- [ ] **Step 1: 更新 paths 测试（失败先行）**

paths_test.go 追加：

```go
func TestPathsStageDefaultAndOverride(t *testing.T) {
	def := New("/data/adb/utsusemi")
	if def.Stage != "/data/local/tmp/utsusemi" {
		t.Fatalf("default stage: %s", def.Stage)
	}
	if def.GadgetConfig() != "/data/local/tmp/utsusemi/gadget.json" {
		t.Fatalf("gadget config moved to stage: %s", def.GadgetConfig())
	}
	ov := NewStage("/r", "/s")
	if ov.Stage != "/s" || ov.GadgetConfig() != "/s/gadget.json" {
		t.Fatalf("override: %+v", ov)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd ctl && go test ./internal/core/`
Expected: FAIL（Stage 字段不存在）

- [ ] **Step 3: 实现 Paths.Stage**

```go
// Paths 汇聚所有数据落点；Root/Stage 可被 --data-root/--stage 覆盖以便测试
type Paths struct {
	Root  string // 控制区（root 才能读）
	Stage string // 发布区（app 进程可读，zygisk 从此加载）
}

const DefaultStage = "/data/local/tmp/" + ModuleID

func New(root string) Paths { return Paths{Root: root, Stage: DefaultStage} }
func NewStage(root, stage string) Paths { return Paths{Root: root, Stage: stage} }
func Default() Paths { return New(DefaultRoot) }

func (p Paths) EnsureStage() error { return os.MkdirAll(p.Stage, 0o755) }
```

`GadgetConfig()` 返回 `filepath.Join(p.Stage, "gadget.json")`（删除原 Root 下的实现）。

- [ ] **Step 4: gadgetcfg 测试更新（发布行为）**

config_test.go 中 `TestApplyGeneratesZygiskFridaCompatibleConfig` 断言路径从 `p.GadgetDir()+"/..."` 改为 `p.Stage+"/...`，并新增发布复制断言：

```go
func TestApplyPublishesGadgetToStage(t *testing.T) {
	p := core.NewStage(t.TempDir(), filepath.Join(t.TempDir(), "stage"))
	p.Ensure()
	seedGadget(t, p, "official_1.0_arm64.so")
	r := Rules{Rules: []Rule{{AppName: "com.a", Enabled: true}}}
	r.Save(p)
	if err := Apply(p); err != nil {
		t.Fatal(err)
	}
	staged := filepath.Join(p.Stage, "official_1.0_arm64.so")
	if _, err := os.Stat(staged); err != nil {
		t.Fatal("gadget not published to stage")
	}
	raw, _ := os.ReadFile(p.GadgetConfig())
	var c Config
	json.Unmarshal(raw, &c)
	if c.Targets[0].InjectedLibraries[0].Path != staged {
		t.Fatalf("path should point to stage: %+v", c.Targets[0])
	}
	// 二次 apply（sha 相同）幂等不报错
	if err := Apply(p); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 5: 实现 Apply 发布逻辑**

在 gadgetcfg `resolve` 中：查 manifest 得 key 后，计算源文件 sha256 与 `p.Stage/key` 比对，不同则复制（0644），返回 `filepath.Join(p.Stage, key)`；Apply 开头 `p.EnsureStage()`。

- [ ] **Step 6: `--stage` 旗标 + 全量测试**

root.go：`root.PersistentFlags().StringVar(&stageDir, "stage", core.DefaultStage, "发布区目录")`，`paths()` 改为 `core.NewStage(dataRoot, stageDir)`。

Run: `cd ctl && go test ./...`
Expected: PASS（注意修复 `TestApplyFailsWhenGadgetMissing` 等旧测试的 Stage 派生路径）

- [ ] **Step 7: Commit**

```bash
git add ctl && git commit -m "feat(ctl): staging publish area for app-readable gadget"
```

---

### Task 2: `boot` 子命令（开机入口，尊重 autostart）

**Files:**
- Modify: `ctl/cmd/server.go`（或新建 `ctl/cmd/boot.go`）
- Test: `ctl/cmd/cli_test.go`

**Interfaces:**
- Consumes: `srv.Manager`、`gadgetcfg.Apply`
- Produces: `utsusemi-ctl boot` → data: `{"autostart": bool, "server": srv.Status}`。行为：读 settings；`autostart=true` 则 `srv.Start()`；随后无论是否启动都执行 `gadgetcfg.Apply(paths())`（staging 同步，失败不影响 server 结果但记入 Note）；`autostart=false` 输出 skipped。

- [ ] **Step 1: 失败测试**

cli_test.go 追加：

```go
func TestBootRespectsAutostart(t *testing.T) {
	root := t.TempDir()
	env := runJSON(t, "--data-root", root, "boot")
	if !env.OK {
		t.Fatal(env.Error)
	}
	raw, _ := json.Marshal(env.Data)
	var d struct {
		Autostart bool `json:"autostart"`
		Server    struct{ Running bool } `json:"server"`
	}
	json.Unmarshal(raw, &d)
	// 默认 autostart=true，但无激活二进制 → server 不运行，命令仍成功
	if !d.Autostart || d.Server.Running {
		t.Fatalf("boot data: %+v", d)
	}
	// 关闭 autostart 后 boot 直接跳过
	runJSON(t, "--data-root", root, "server", "set", "--autostart=false")
	env = runJSON(t, "--data-root", root, "boot")
	raw, _ = json.Marshal(env.Data)
	json.Unmarshal(raw, &d)
	if d.Autostart {
		t.Fatal("autostart off but boot attempted start")
	}
}
```

- [ ] **Step 2: 确认失败 → 实现 `boot.go` → 测试通过**

```go
func newBootCmd() *cobra.Command {
	return &cobra.Command{
		Use: "boot",
		RunE: func(c *cobra.Command, _ []string) error {
			s, err := core.LoadSettings(paths())
			if err != nil {
				return fail(c, err)
			}
			note := ""
			var st srv.Status
			if s.Server.Autostart {
				st, err = srv.New(paths()).Start()
				if err != nil {
					note = err.Error()
				}
			}
			// gadget 发布区同步失败不阻断 boot
			if aerr := gadgetcfg.Apply(paths()); aerr != nil && note == "" {
				note = "gadget: " + aerr.Error()
			}
			emit(c, Envelope{OK: true, Data: map[string]any{
				"autostart": s.Server.Autostart, "server": st, "note": note,
			}})
			return nil
		},
	}
}
```

root.go 注册 `newBootCmd()`。Run: `go test ./...` → PASS。

- [ ] **Step 3: Commit**

```bash
git add ctl && git commit -m "feat(ctl): boot command honoring autostart and staging sync"
```

---

### Task 3: native 剥离 Riru + 配置路径改 staging

**Files:**
- Delete: `native/zygisk/main_riru.cpp`、`native/zygisk/include/riru_config.h`、`native/zygisk/Android.mk`、`native/zygisk/Application.mk`
- Modify: `native/zygisk/inject.cpp`（module_dir 常量）
- Modify: `native/zygisk/config.cpp`（config 文件名 `config.json` → `gadget.json`；同时移除 simple-config 分支或保留均可——选择移除 `target_packages`/`injected_libraries` 简易模式，ctl 不再生成）

**Interfaces:**
- Consumes: 无
- Produces: zygisk so 只从 `/data/local/tmp/utsusemi/gadget.json` 读取配置（路径常量 `kConfigDir = "/data/local/tmp/utsusemi"`，定义于 `config.h`）

- [ ] **Step 1: 删除 riru 与简易配置**

```bash
cd native/zygisk
git rm main_riru.cpp include/riru_config.h Android.mk Application.mk
```

`config.cpp`：删除 `parse_injected_libraries`、`load_simple_config`、`split` 及 `load_config` 中的 simple 分支；`load_advanced_config` 内文件名改 `gadget.json`；`load_config(module_dir, app_name)` 签名保留。

`inject.cpp:111` 附近：`std::string module_dir = std::string("/data/local/tmp/utsusemi");`

`config.h` 增加：

```cpp
// gadget 注入配置所在目录（staging，app 进程可读）
constexpr const char *kConfigDir = "/data/local/tmp/utsusemi";
```

（`inject.cpp` 改用 `kConfigDir`。）

- [ ] **Step 2: 语法级验证（宿主编译器粗查）**

Run: `g++ -fsyntax-only -std=c++17 native/zygisk/config.cpp 2>&1 | head -5 || true`（宿主无 android 头会报少量环境错，仅确认无纯语法/结构错；真正编译在 Task 4）

- [ ] **Step 3: Commit**

```bash
git add -A native && git commit -m "refactor(native): drop riru flavor, read gadget.json from staging"
```

---

### Task 4: vendor dobby + CMake 构建 zygisk so

**Files:**
- Create: `native/dobby/`（源码自 `/tmp/opencode/refs/dobby-android/dobby/src/main/dobby` 拷入，含 LICENSE）
- Create: `native/CMakeLists.txt`
- Create: `scripts/build-zygisk.sh`

**Interfaces:**
- Consumes: Task 3 产物源码
- Produces: `build/zygisk/{armeabi-v7a,arm64-v8a,x86,x86_64}.so`（供 Task 6 打包消费）

- [ ] **Step 1: vendor dobby**

```bash
mkdir -p native/dobby
cp -r /tmp/opencode/refs/dobby-android/dobby/src/main/dobby/. native/dobby/
ls native/dobby/CMakeLists.txt native/dobby/LICENSE
```

- [ ] **Step 2: 根 CMakeLists**

`native/CMakeLists.txt`：

```cmake
cmake_minimum_required(VERSION 3.22.1)
project(utsusemi)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# dobby: 静态库
set(DOBBY_GENERATE_SHARED OFF CACHE BOOL "" FORCE)
set(DOBBY_BUILD_SHARED_LIBRARY OFF CACHE BOOL "" FORCE)
add_subdirectory(dobby)

set(XDL_SOURCES zygisk/xdl/xdl.c zygisk/xdl/xdl_iterate.c zygisk/xdl/xdl_linker.c
                 zygisk/xdl/xdl_lzma.c zygisk/xdl/xdl_util.c)

add_library(utsusemi_zygisk SHARED
        zygisk/main_zygisk.cpp
        zygisk/inject.cpp
        zygisk/config.cpp
        zygisk/child_gating.cpp
        zygisk/remapper.cpp
        ${XDL_SOURCES})

target_include_directories(utsusemi_zygisk PRIVATE
        zygisk zygisk/xdl/include zygisk/include dobby/include/include)

target_compile_options(utsusemi_zygisk PRIVATE
        -fno-exceptions -fno-rtti -fvisibility=hidden -fvisibility-inlines-hidden)

target_link_libraries(utsusemi_zygisk dobby log)
```

（dobby include 路径以其源码树实际布局为准，构建时修正。）

- [ ] **Step 3: 构建脚本**

`scripts/build-zygisk.sh`：

```bash
#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
NDK="${NDK:-/data/.build-tools-cache/Android/ndk/28.2.13676358}"
OUT="$ROOT/build/zygisk"
TOOLCHAIN="$NDK/build/cmake/android.toolchain.cmake"

declare -A ABIS=( [armeabi-v7a]=armv7-a-neon [arm64-v8a]=arm64-v8a [x86]=x86 [x86_64]=x86_64 )
for abi in arm64-v8a armeabi-v7a x86 x86_64; do
  build="$ROOT/build/native/$abi"
  mkdir -p "$build"
  cmake -G Ninja -S "$ROOT/native" -B "$build" \
    -DCMAKE_TOOLCHAIN_FILE="$TOOLCHAIN" \
    -DANDROID_ABI="$abi" -DANDROID_PLATFORM=android-21 \
    -DANDROID_STL=c++_static \
    -DCMAKE_BUILD_TYPE=Release >/dev/null
  cmake --build "$build" -j >/dev/null
  mkdir -p "$OUT"
  cp "$build/libutsusemi_zygisk.so" "$OUT/$abi.so"
  file "$OUT/$abi.so"
done
```

- [ ] **Step 4: 构建并修正直至四 ABI 产出**

Run: `chmod +x scripts/build-zygisk.sh && scripts/build-zygisk.sh`
Expected: 4 个 so；`file` 显示对应架构 ELF shared object。预期需手工修正：dobby 的 CMake 目标名/头文件路径、`zygisk.h` 对 `apiTable` 的引用、`log.h` 的 `__android_log_print`（需 `-llog` 已加）。**编译报错逐个修，不改注入逻辑**。

- [ ] **Step 5: Commit**

```bash
git add native scripts && git commit -m "build(native): cmake build with vendored dobby, four abis"
```

---

### Task 5: 模块模板（customize/service/action/uninstall/verify + webroot 占位）

**Files:**
- Create: `template/module.prop`
- Create: `template/customize.sh`
- Create: `template/service.sh`
- Create: `template/action.sh`
- Create: `template/uninstall.sh`
- Create: `template/verify.sh`
- Create: `template/webroot/index.html`

**Interfaces:**
- Consumes: Task 4 的 `build/zygisk/*.so`、Phase 1 的 `build/ctl/<abi>/utsusemi-ctl`（打包布局：zip 内 `lib/<abi>/utsusemi-ctl`、`zygisk/<abi>.so`）
- Produces: 可被 Task 6 组装的安装脚本集

- [ ] **Step 1: module.prop（version 由打包注入占位 `@VERSION@`/`@VERSION_CODE@`）**

```text
id=utsusemi
name=Utsusemi-Yadori
version=@VERSION@
versionCode=@VERSION_CODE@
author=utsusemi
description=Frida manager: server + zygisk gadget injection (● status)
updateJson=
```

- [ ] **Step 2: verify.sh（sha256 校验安装，沿用 ZygiskFrida 方案）**

```sh
TMPDIR_FOR_VERIFY="$TMPDIR/.vunzip"
mkdir "$TMPDIR_FOR_VERIFY"

abort_verify() {
  ui_print "*********************************************************"
  ui_print "! $1"
  ui_print "! This zip may be corrupted, please try downloading again"
  abort    "*********************************************************"
}

# extract <zip> <file> <target dir> <junk paths>
extract() {
  zip=$1; file=$2; dir=$3; junk_paths=$4
  [ -z "$junk_paths" ] && junk_paths=false
  opts="-o"; [ $junk_paths = true ] && opts="-oj"
  if [ $junk_paths = true ]; then
    file_path="$dir/$(basename "$file")"
    hash_path="$TMPDIR_FOR_VERIFY/$(basename "$file").sha256sum"
  else
    file_path="$dir/$file"
    hash_path="$TMPDIR_FOR_VERIFY/$file.sha256sum"
  fi
  unzip $opts "$zip" "$file" -d "$dir" >&2
  [ -f "$file_path" ] || abort_verify "$file not exists"
  unzip $opts "$zip" "$file.sha256sum" -d "$TMPDIR_FOR_VERIFY" >&2
  [ -f "$hash_path" ] || abort_verify "$file.sha256sum not exists"
  (echo "$(cat "$hash_path")  $file_path" | sha256sum -c -s -) || abort_verify "Failed to verify $file"
  ui_print "- Verified $file" >&1
}
```

- [ ] **Step 2.5: customize.sh（SKIPUNZIP=1；按 ABI 选装；webroot 清理重拷；数据目录仅初始化不覆盖）**

```sh
SKIPUNZIP=1

MODULE_ID=utsusemi
DATA_DIR=/data/adb/utsusemi
STAGE_DIR=/data/local/tmp/utsusemi

[ "$ARCH" != "arm" ] && [ "$ARCH" != "arm64" ] && [ "$ARCH" != "x86" ] && [ "$ARCH" != "x64" ] && \
  abort "! Unsupported platform: $ARCH"
ui_print "- Device platform: $ARCH"

ui_print "- Extracting verify.sh"
unzip -o "$ZIPFILE" 'verify.sh' -d "$TMPDIR" >&2
. "$TMPDIR/verify.sh"

ui_print "- Installing module files"
extract "$ZIPFILE" 'module.prop' "$MODPATH"
extract "$ZIPFILE" 'service.sh' "$MODPATH"
extract "$ZIPFILE" 'action.sh' "$MODPATH"
extract "$ZIPFILE" 'uninstall.sh' "$MODPATH"

# ctl 与 zygisk so 按设备 ABI 选装（zip 内 lib/<abi>/、zygisk/<abi>.so）
MAGISK_ABI="$ARCH"
[ "$ARCH" = "x64" ] && MAGISK_ABI=x86_64
CTL_SRC="lib/$MAGISK_ABI/utsusemi-ctl"
mkdir -p "$MODPATH/bin" "$MODPATH/zygisk"
extract "$ZIPFILE" "$CTL_SRC" "$MODPATH/bin" true
mv "$MODPATH/bin/utsusemi-ctl" "$MODPATH/bin/utsusemi-ctl.tmp"
extract "$ZIPFILE" "zygisk/$MAGISK_ABI.so" "$MODPATH/zygisk" true

# 32 位侧 zygisk so（64 位设备同时装 32 位，兼容混合架构应用）
if [ "$IS64BIT" = true ]; then
  ABI32=armeabi-v7a
  [ "$ARCH" = "x64" ] && ABI32=x86
  [ "$ARCH" = "x86" ] && ABI32=x86
  extract "$ZIPFILE" "zygisk/$ABI32.so" "$MODPATH/zygisk" true
fi

# webroot：先清后装，防升级残留旧 hash 文件（KSU 自动处理其权限/上下文，勿 set_perm）
ui_print "- Installing webroot"
rm -rf "$MODPATH/webroot"
unzip -o "$ZIPFILE" 'webroot/*' -d "$MODPATH" >&2

# 数据目录：仅首次初始化，绝不覆盖用户 settings/manifest/frida-bin
ui_print "- Preparing data dirs"
"$MODPATH/bin/utsusemi-ctl.tmp" version >/dev/null 2>&1 && mv "$MODPATH/bin/utsusemi-ctl.tmp" "$MODPATH/bin/utsusemi-ctl"
set_perm "$MODPATH/bin/utsusemi-ctl" 0 0 0755
set_perm_recursive "$MODPATH/zygisk" 0 0 0755 0755
set_perm_recursive "$MODPATH" 0 0 0755 0644
mkdir -p "$DATA_DIR" "$STAGE_DIR"
chmod 0755 "$DATA_DIR" "$STAGE_DIR"

ui_print "- Done! Use WebUI or action button to manage frida."
```

（注意：`set_perm_recursive $MODPATH 0755 0644` 需在 bin/zygisk 单独 set_perm 之后或调整顺序避免覆盖执行位——实现时以最终实测为准，先递归 0644 再对 bin/zygisk 显式 0755。）

- [ ] **Step 3: service.sh / action.sh / uninstall.sh**

service.sh：

```sh
#!/system/bin/sh
MODDIR=${0%/*}
exec 2>"$MODDIR/logs/service.log"
mkdir -p "$MODDIR/logs"
# ctl 内部判断 autostart 并完成 staging 同步；late_start 非阻塞
"$MODDIR/bin/utsusemi-ctl" boot >/dev/null 2>&1 &
```

action.sh：

```sh
#!/system/bin/sh
MODDIR=${0%/*}
CTL="$MODDIR/bin/utsusemi-ctl"
if "$CTL" --json server status 2>/dev/null | grep -q '"running":true'; then
  "$CTL" server stop
else
  "$CTL" server start
fi
"$CTL" --json server status 2>/dev/null || true
```

uninstall.sh：

```sh
#!/system/bin/sh
DATA_DIR=/data/adb/utsusemi
STAGE_DIR=/data/local/tmp/utsusemi
rm -rf "$STAGE_DIR"
# 保留用户数据二进制库由用户决定：默认保留 DATA_DIR（含已下载 frida 与配置）
echo "utsusemi: stage cleaned; data kept at $DATA_DIR"
```

- [ ] **Step 4: webroot 占位**

```html
<!DOCTYPE html>
<html lang="zh-CN"><head><meta charset="utf-8"><title>Utsusemi-Yadori</title></head>
<body style="background:#111;color:#eee;font-family:sans-serif;padding:2rem">
<h1>Utsusemi-Yadori 蝉宿</h1>
<p>WebUI 将在 Phase 3 上线。请先使用 Action 按钮或 utsusemi-ctl CLI。</p>
</body></html>
```

- [ ] **Step 5: shellcheck/语法自检 + Commit**

Run: `for f in template/*.sh; do sh -n "$f" && echo "OK $f"; done`
```bash
git add template && git commit -m "feat(template): module installer scripts with sha256 verify"
```

---

### Task 6: 打包流水线 build.sh + zip 结构断言

**Files:**
- Create: `scripts/build.sh`
- Create: `scripts/assert-zip.sh`（结构断言，供 CI/本地校验）

**Interfaces:**
- Consumes: `scripts/build-ctl.sh`、`scripts/build-zygisk.sh`、`template/`
- Produces: `dist/utsusemi-<version>.zip`（flashable；内含全部 `.sha256sum`；`.gitignore` 增加 `dist/`）

- [ ] **Step 1: build.sh**

```bash
#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="${1:-v0.1.0}"
VERSION_CODE="${VERSION_CODE:-1}"
cd "$ROOT"

echo "== ctl =="
scripts/build-ctl.sh
echo "== zygisk =="
scripts/build-zygisk.sh

STAGE="$ROOT/build/package"
DIST="$ROOT/dist"
rm -rf "$STAGE"; mkdir -p "$STAGE/lib" "$STAGE/zygisk" "$DIST"

cp -r template/. "$STAGE/"
sed -i "s/@VERSION@/$VERSION/; s/@VERSION_CODE@/$VERSION_CODE/" "$STAGE/module.prop"

for abi in arm64-v8a armeabi-v7a x86 x86_64; do
  mkdir -p "$STAGE/lib/$abi"
  cp "build/ctl/$abi/utsusemi-ctl" "$STAGE/lib/$abi/utsusemi-ctl"
  cp "build/zygisk/$abi.so" "$STAGE/zygisk/$abi.so"
done

# 全部文件生成 sha256sum（LF 规范化后计算文本文件）
cd "$STAGE"
find . -type f ! -name '*.sha256sum' -print0 | while IFS= read -r -d '' f; do
  sum=$(sha256sum "$f" | cut -d' ' -f1)
  echo "$sum" > "$f.sha256sum"
done
# CRLF 防御：文本脚本必须是 LF
find . -name '*.sh' -exec file {} \; | grep -i crlf && { echo "CRLF detected!"; exit 1; } || true

ZIP="$DIST/utsusemi-$VERSION.zip"
rm -f "$ZIP"
zip -r -X "$ZIP" . >/dev/null
echo "built: $ZIP"
unzip -l "$ZIP" | tail -5
```

- [ ] **Step 2: assert-zip.sh（结构门禁）**

```bash
#!/usr/bin/env bash
# 校验 dist zip 结构完整（customize 引用的关键文件均存在且带 sha256）
set -euo pipefail
ZIP="${1:?usage: assert-zip.sh <zip>}"
need=(
  module.prop customize.sh service.sh action.sh uninstall.sh verify.sh
  webroot/index.html
  zygisk/arm64-v8a.so zygisk/armeabi-v7a.so zygisk/x86.so zygisk/x86_64.so
  lib/arm64-v8a/utsusemi-ctl lib/armeabi-v7a/utsusemi-ctl
  lib/x86/utsusemi-ctl lib/x86_64/utsusemi-ctl
)
for f in "${need[@]}"; do
  unzip -l "$ZIP" "$f" | grep -q "$f" || { echo "MISSING: $f"; exit 1; }
  unzip -l "$ZIP" "$f.sha256sum" | grep -q "$f.sha256sum" || { echo "MISSING: $f.sha256sum"; exit 1; }
done
# module.prop 注入检查
unzip -p "$ZIP" module.prop | grep -q '@VERSION@' && { echo "module.prop not rendered"; exit 1; } || true
echo "zip structure OK"
```

- [ ] **Step 3: 全流程跑通 + 断言 + Commit**

Run: `scripts/build.sh v0.1.0 && scripts/assert-zip.sh dist/utsusemi-v0.1.0.zip`
Expected: zip structure OK。`.gitignore` 追加 `dist/`。

```bash
git add scripts .gitignore && git commit -m "build: full packaging pipeline with structure assertions"
```

---

### Task 7: README 更新 + 收尾验证

**Files:**
- Modify: `README.md`

- [ ] **Step 1: README 补充构建/安装/使用说明**（构建三脚本、zip 安装、CLI 速查、staging 说明、已知限制：非 arm64 ABI 的 ctl DNS）
- [ ] **Step 2: 终验**

Run: `cd ctl && go test ./... && cd .. && scripts/build.sh v0.1.0 && scripts/assert-zip.sh dist/utsusemi-v0.1.0.zip`
Expected: 全绿 + zip OK

- [ ] **Step 3: Commit**

```bash
git add README.md && git commit -m "docs: phase 2 build and usage guide"
```

---

## Self-Review 结论

- **Spec 覆盖**：§4.1 发布区（Task 1/2）、§7 模板与安装含 webroot 清理（Task 5）、§8 native 构建与打包（Task 4/6）；§10 风险 1 的 SELinux 决策已落地为 staging 方案。Phase 3 范围（Gin/前端/`api` 子命令）未混入。
- **占位符扫描**：Task 4 Step 4 说明"编译报错逐个修"是构建调参预期而非占位；其余步骤均含完整代码。
- **类型一致性**：`Paths.NewStage`/`EnsureStage`/`GadgetConfig` 变更在 Task 1 集中定义；`build/zygisk/<abi>.so` 与 `lib/<abi>/utsusemi-ctl` 布局在 Task 4/5/6 间一致。
