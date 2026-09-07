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
