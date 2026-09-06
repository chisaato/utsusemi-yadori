# Utsusemi-Yadori 蝉宿

一个功能超丰富的 Frida 管理模块，支持 Magisk/KSU/SukiSU/APatch/FolkPatch：

- frida-server 的启动与停止（开机自启、崩溃检测、状态回写 module.prop）
- frida-gadget 按应用注入（Zygisk，支持延时注入与 child gating）
- 二进制零内置：安装后在线下载（官方 / [Florida](https://github.com/Ylarod/Florida) / [undetected-frida](https://github.com/ultrafunkamsterdam/undetected-frida)），或导入自编译版本
- WebUI（Phase 3 上线；当前为占位页）

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
utsusemi-ctl boot        # service.sh 开机入口（尊重 autostart + 同步发布区）
utsusemi-ctl status
```

所有子命令支持 `--json`（与后续 WebUI REST 同构）；`--data-root`/`--stage` 可重定向便于测试。

## 已知限制

- 非arm64 设备的 ctl 为 GOOS=linux 纯静态构建，其纯 Go DNS 解析器在无 resolv.conf 的设备上可能失败——可用镜像源或 USB 导入绕过。
- Zygisk 需要 Magisk 内建 Zygisk 或 ZygiskNext；KSU 系管理器需自带 Zygisk 实现（SukiSU 可用）。
- gadget 注入依赖 `/data/local/tmp` 可读性（SELinux），沿用 ZygiskFrida 验证方案。
