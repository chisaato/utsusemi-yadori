import { ApiError } from './errors'
import { mockCtlRaw, mockRequest } from './mock'
import type { Envelope } from './types'

export type TransportMode = 'ksu' | 'rest'

/** KSU/SukiSU/APatch 管理器注入 WebView 的全局对象（ksu / kpu） */
interface KsuLike {
  exec: (...args: unknown[]) => void
}

function detectKsu(): KsuLike | null {
  const w = window as unknown as Record<string, unknown>
  const k = w.ksu ?? w.kpu
  return k && typeof (k as KsuLike).exec === 'function' ? (k as KsuLike) : null
}

export const ksu = detectKsu()
export const mode: TransportMode = ksu ? 'ksu' : 'rest'

/** 顶栏传输模式徽标：MOCK 仅存在于 dev（build 时被常量折叠掉） */
export const transportLabel: string =
  import.meta.env.DEV && import.meta.env.VITE_LIVE !== '1' ? 'MOCK' : mode === 'ksu' ? 'KSU' : 'REST'

/* ------------------------------------------------------------------ */
/* ksu.exec 适配                                                       */
/* ------------------------------------------------------------------ */

let cbSeq = 0

/**
 * KernelSU 系管理器注入的原生签名（KernelSU/SukiSU 一致）：
 *   exec(cmd) / exec(cmd, cbName) / exec(cmd, optionsJson, cbName)
 * optionsJson 为 JSON 字符串（仅支持 cwd/env），原生无 timeout 参数；
 * timeoutSec 仅用于本地看门狗。参数个数/类型不匹配会同步抛 "method not found"。
 */
function ksuExec(cmd: string, timeoutSec = 300): Promise<{ errno: number; stdout: string; stderr: string }> {
  return new Promise((resolve, reject) => {
    const cbName = `__utsusemi_exec_${Date.now().toString(36)}_${++cbSeq}`
    const w = window as unknown as Record<string, unknown>
    let timer = 0 as unknown as ReturnType<typeof setTimeout>

    // 无论成功失败都清理全局回调与看门狗
    const cleanup = () => {
      clearTimeout(timer)
      delete w[cbName]
    }
    // 管理器不回调时兜底（timeout 秒 + 60s 缓冲）
    timer = setTimeout(() => {
      cleanup()
      reject(new ApiError('命令执行超时（ksu.exec 未返回）'))
    }, (timeoutSec + 60) * 1000)

    // 个别实现 stdout 为行数组
    w[cbName] = (errno: number, stdout: string | string[], stderr: string) => {
      const out = Array.isArray(stdout) ? stdout.join('\n') : String(stdout ?? '')
      cleanup()
      resolve({ errno, stdout: out, stderr: String(stderr ?? '') })
    }

    try {
      ksu?.exec(cmd, cbName)
    } catch {
      try {
        // 兜底：三参重载（optionsJson 必须为字符串）
        ksu?.exec(cmd, '{}', cbName)
      } catch (e) {
        cleanup()
        reject(new ApiError('ksu.exec 调用失败：' + String(e)))
      }
    }
  })
}

/** 值仅含常规字符时裸传，否则加单引号（payload 由契约保证不含单引号，固定单引号包裹） */
function shellArg(v: string): string {
  return /^[\w.:@%+=,/-]+$/.test(v) ? v : `'${v.replace(/'/g, `'\\''`)}'`
}

export interface CtlSpec {
  /** api 子命令，如 'status'、'bin/download'、'tasks/<id>' */
  sub: string
  flags?: Record<string, string | number | undefined>
  payload?: unknown
}

/** utsusemi-ctl 二进制绝对路径 */
const CTL_BIN = '/data/adb/modules/utsusemi/bin/utsusemi-ctl'

export function buildCtlCmd(spec: CtlSpec): string {
  const parts = [CTL_BIN, 'api', spec.sub]
  for (const [k, v] of Object.entries(spec.flags ?? {})) {
    // 空值旗标直接跳过（如可选 version）
    if (v === undefined || v === null || v === '') continue
    parts.push(`--${k}`, shellArg(String(v)))
  }
  if (spec.payload !== undefined) parts.push('--payload', `'${JSON.stringify(spec.payload)}'`)
  return parts.join(' ')
}

/** 契约：stdout 末行 JSON；从后往前容错解析（跳过杂散告警行） */
function parseEnvelope<T>(stdout: string): Envelope<T> {
  const lines = stdout.split('\n').map((l) => l.trim()).filter(Boolean)
  for (let i = lines.length - 1; i >= 0; i--) {
    try {
      const j = JSON.parse(lines[i]) as Envelope<T>
      if (j && typeof j === 'object' && typeof j.ok === 'boolean') return j
    } catch {
      // 继续向上寻找 JSON 行
    }
  }
  throw new ApiError('utsusemi-ctl 输出中未找到 JSON 响应')
}

/* ------------------------------------------------------------------ */
/* REST 适配                                                           */
/* ------------------------------------------------------------------ */

export interface RestSpec {
  method: 'GET' | 'POST' | 'PUT' | 'DELETE'
  path: string
  body?: unknown
  /** multipart 表单（导入上传） */
  form?: FormData
}

const TOKEN_KEY = 'utsusemi_token'

async function restFetch<T>(rest: RestSpec): Promise<Envelope<T>> {
  const headers: Record<string, string> = {}
  // token 每次实时读取，支持用户重新从入口进入后立即生效
  const token = localStorage.getItem(TOKEN_KEY)
  if (token) headers.Authorization = `Bearer ${token}`

  let body: BodyInit | undefined
  if (rest.form) body = rest.form
  else if (rest.body !== undefined) {
    headers['Content-Type'] = 'application/json'
    body = JSON.stringify(rest.body)
  }

  let res: Response
  try {
    res = await fetch(rest.path, { method: rest.method, headers, body })
  } catch {
    throw new ApiError('网络请求失败，无法连接服务')
  }

  if (res.status === 401) {
    const err = new ApiError('访问令牌无效或已过期，请从带 ?token= 的入口链接重新进入', 401, true)
    // 全局弹窗提示（AppLayout 监听）
    window.dispatchEvent(new CustomEvent('utsusemi:unauthorized', { detail: err.message }))
    throw err
  }

  try {
    return (await res.json()) as Envelope<T>
  } catch {
    throw new ApiError('服务返回了无法解析的内容')
  }
}

/* ------------------------------------------------------------------ */
/* 统一入口                                                            */
/* ------------------------------------------------------------------ */

/**
 * 双传输统一请求：ksu 模式走 utsusemi-ctl api 子命令，REST 模式走 fetch。
 * 返回已解包的 data；ok:false / 传输失败均抛 ApiError。
 */
export async function request<T>(rest: RestSpec | null, ctl: CtlSpec | null): Promise<T> {
  // DEV mock 分支：build 时 import.meta.env.DEV 被 replace 成 false，整段连同 mock 模块被摇树剔除
  if (import.meta.env.DEV && import.meta.env.VITE_LIVE !== '1') {
    return mockRequest<T>(rest, ctl)
  }

  let env: Envelope<T>

  if (mode === 'ksu') {
    if (!ctl) throw new ApiError('该操作在 KSU 管理器模式下不可用')
    const { errno, stdout, stderr } = await ksuExec(buildCtlCmd(ctl))
    if (errno !== 0) {
      // 取 stderr 末行作为可读提示
      const hint = stderr.trim().split('\n').filter(Boolean).pop() ?? ''
      throw new ApiError(hint || `命令执行失败（退出码 ${errno}）`)
    }
    env = parseEnvelope(stdout)
  } else {
    if (!rest) throw new ApiError('该操作在 PC 远程模式下不可用')
    env = await restFetch<T>(rest)
  }

  if (!env.ok) throw new ApiError(env.error ?? '未知错误')
  return env.data as T
}

/**
 * 直接 exec utsusemi-ctl 原生命令（非 api 信封），返回 stdout 文本。
 * 用于 ksu 模式下 `web start --port N` / `web stop` 这类控制命令。
 * 仅 ksu 传输可用；REST 模式调用方需自行走对应 REST 端点。
 */
export async function execCtlRaw(args: string, timeoutSec = 60): Promise<string> {
  // DEV mock 分支（build 时常量折叠消除）
  if (import.meta.env.DEV && import.meta.env.VITE_LIVE !== '1') {
    return mockCtlRaw(args)
  }
  const { errno, stdout, stderr } = await ksuExec(`${CTL_BIN} ${args}`, timeoutSec)
  if (errno !== 0) {
    const hint = stderr.trim().split('\n').filter(Boolean).pop() ?? ''
    throw new ApiError(hint || `命令执行失败（退出码 ${errno}）`)
  }
  return stdout.trim()
}

/**
 * 入口 token 捕获：?token=xxx → localStorage，并立即从 URL 清除。
 * 必须在 app mount 前调用（main.ts）。
 */
export function captureToken(): void {
  try {
    const url = new URL(window.location.href)
    const token = url.searchParams.get('token')
    if (!token) return
    localStorage.setItem(TOKEN_KEY, token)
    url.searchParams.delete('token')
    window.history.replaceState(null, '', url.pathname + (url.search || '') + (url.hash || ''))
  } catch {
    // 非常规 URL（自定义 scheme 等）直接忽略
  }
}
