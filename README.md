# Utsusemi-Yadori 蝉宿

一个功能超丰富的 Frida 管理模块，支持 Magisk/KSU/SukiSU/APatch/FolkPatch：

- frida-server 的启动与停止（开机自启、崩溃检测、状态回写 module.prop）
- frida-gadget 按应用注入（Zygisk，支持延时注入与 child gating）
- 二进制零内置：安装后在线下载（官方 / [Florida](https://github.com/Ylarod/Florida) / [undetected-frida](https://github.com/ultrafunkamsterdam/undetected-frida)），或导入自编译版本
- 双模 WebUI：管理器内置 WebUI（手机端）+ 按需启动的远程 Web 服务（PC 协同，token 鉴权）

参考项目

- [magisk-frida](https://github.com/virb3/magisk-frida)
- [ZygiskFrida](https://github.com/lico-n/ZygiskFrida)（zygisk 注入层源码基础）

## 构建

依赖：Go ≥1.26、Bun（Phase 3 前端）、Android NDK（默认路径可用 `NDK=` 覆盖）、CMake + Ninja、zip/unzip。

```bash
scripts/build.sh v0.1.0              # 一键：ctl 四 ABI + zygisk so + 模板 → dist/*.zip
scripts/assert-zip.sh dist/*.zip     # zip 结构门禁
```

子构建：`scripts/build-ctl.sh`（Go 交叉编译）、`scripts/build-zygisk.sh`（NDK+CMake）。

## 安装

管理器中刷入 `dist/utsusemi-<version>.zip`。安装脚本带 sha256 校验、按设备 ABI 选装、升级时清理旧 webroot、保留 `/data/adb/utsusemi` 用户数据。

## 目录布局

```
/data/adb/utsusemi/            控制区: settings.json / manifest.json / rules.json / frida-bin/
/data/local/tmp/utsusemi/      发布区: gadget.json + gadget so（app 进程可读，zygisk 加载点）
/data/adb/modules/utsusemi/    模块本体: bin/utsusemi-ctl、zygisk/*.so、webroot/
```

manifest.json 是唯一事实来源（variant/version/arch/sha256），文件名只是 key；用户导入任意命名二进制均可。

## CLI 速查

```
utsusemi-ctl bin sources|download|import|list|use|remove|cleanup
utsusemi-ctl server start|stop|restart|status|set
utsusemi-ctl gadget apps|rules|set|apply
utsusemi-ctl web start|stop|status        # 远程 Web 服务按需启停（--port/--token 可覆盖）
utsusemi-ctl api <subcommand>             # WebUI 桥：与 REST 同构的 JSON 输出
utsusemi-ctl boot        # service.sh 开机入口（尊重 autostart + 同步发布区）
utsusemi-ctl status
```

所有子命令支持 `--json`（与 WebUI REST 同构，契约见 `docs/api-contract.md`）；`--data-root`/`--stage` 可重定向便于测试。

## WebUI

一套前端两处消费：管理器内置 WebUI（`webroot/`，手机端）与远程 Web 服务（同一前端 embed 进 ctl，PC 浏览器访问）。传输层自动探测：KSU 环境走 `ksu.exec` 直调 ctl，浏览器走 REST + token。

**手机端**：管理器（KSU/SukiSU/APatch）打开本模块 WebUI 即用，无需任何配置。

**PC 协同（远程 Web）**：

```bash
adb shell /data/adb/modules/utsusemi/bin/utsusemi-ctl web start   # 启动，输出端口与 token
adb forward tcp:23333 tcp:23333                                   # 端口转发
# 浏览器打开 http://127.0.0.1:23333/?token=<输出的token>
```

- token 自动生成并写回 settings（`web start --token` 可固定）；入口 URL 带 token，前端捕获后存 localStorage
- 局域网内任何人都可访问该服务（默认 0.0.0.0 监听）——**用完即 `web stop`**，WebUI 远程协同页有同样警示
- 同一 WLAN 下也可不转发直接访问 `http://<手机IP>:23333/?token=...`

## 已知限制

- 非arm64 设备的 ctl 为 GOOS=linux 纯静态构建，其纯 Go DNS 解析器在无 resolv.conf 的设备上可能失败——可用镜像源或 USB 导入绕过。
- Zygisk 需要 Magisk 内建 Zygisk 或 ZygiskNext；KSU 系管理器需自带 Zygisk 实现（SukiSU 可用）。
- gadget 注入依赖 `/data/local/tmp` 可读性（SELinux），沿用 ZygiskFrida 验证方案。
