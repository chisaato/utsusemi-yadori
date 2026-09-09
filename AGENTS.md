# Utsusemi-Yadori

本模块面向 Magisk/SukiSU 等环境

## 前端/WebUI

模块的 WebUI 前端使用 Vue+Bun 工具链编译

代码风格要求

if 后必须使用大括号,即便一行

反例:

```typescript
if (!t) return ''
if (t.state === 'error') return t.error ?? '任务失败'
const p = progress.value
if (!p) return t.detail ?? ''
```

正例
```typescript
if (!f) {
    message.warning('请先选择文件')
}
```

## Cloned Dependency Source

Read-only dependency source repositories are available under
`.slim/clonedeps/repos/` for inspection. Do not edit these clones.

- `.slim/clonedeps/repos/frida__frida/` - `frida/frida` at HEAD(浅克隆);主仓库入口与子模块指针,源码在下列独立子仓库。
- `.slim/clonedeps/repos/frida__frida-gum/` - `frida/frida-gum` at HEAD(浅克隆);Frida 运行时源码,`gum/backend-linux/`(gumprocess-linux.c、gumandroid.c 等)是模块枚举/注入机制的核心。
- `.slim/clonedeps/repos/frida__frida-core/` - `frida/frida-core` at HEAD(浅克隆);frida-server、注入器与 spawn 逻辑源码。
