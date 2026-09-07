/**
 * DEV 专用 mock（utsusemi-mock）。
 * 仅被 client.ts 中 import.meta.env.DEV 分支引用，build 产物中被摇树剔除。
 * 本模块必须保持零副作用（纯声明 + 函数导出），否则会被强制打进产物。
 */
import type { CtlSpec, RestSpec } from './client'
import type { AppItem, Binary, Rules, ServerSettings, Status, SrvStatus, Task } from './types'

const sleep = (ms: number) => new Promise<void>((r) => setTimeout(r, ms))

/* ------------------------------------------------------------------ */
/* 数据集                                                              */
/* ------------------------------------------------------------------ */

function mkBinary(b: Partial<Binary> & Pick<Binary, 'file' | 'variant' | 'version' | 'elf_type'>): Binary {
  return {
    arch: 'arm64',
    source: `https://github.com/frida/frida/releases/tag/${b.version}`,
    sha256: 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
    size: 24_000_000,
    added_at: '2026-09-06T21:14:00+08:00',
    missing: false,
    ...b,
  }
}

interface MockTask {
  id: string
  created: number
  kind: 'download' | 'import'
  variant: string
  version: string
  type: string
  fail: boolean
  done: boolean
}

const db = {
  running: true,
  pid: 4173,
  startedAt: Date.now() - (3600 * 1000 + 245_000),
  note: '',
  servers: [
    mkBinary({ file: 'official_17.2.14_arm64', variant: 'official', version: '17.2.14', elf_type: 'exec', size: 24_312_832 }),
    mkBinary({ file: 'florida_16.7.19_arm64', variant: 'florida', version: '16.7.19', elf_type: 'exec', size: 23_884_800, added_at: '2026-09-01T10:02:00+08:00' }),
    mkBinary({ file: 'undetected_16.6.1_arm64', variant: 'undetected', version: '16.6.1', elf_type: 'exec', missing: true }),
  ],
  gadgets: [
    mkBinary({ file: 'official_17.2.14_arm64.so', variant: 'official', version: '17.2.14', elf_type: 'dyn', size: 24_312_832 }),
    mkBinary({ file: 'florida_16.7.19_arm64.so', variant: 'florida', version: '16.7.19', elf_type: 'dyn', size: 23_884_800, missing: true, added_at: '2026-09-01T10:02:00+08:00' }),
  ],
  sources: [
    { variant: 'official', versions: ['17.2.14', '17.2.12', '17.1.9', '17.0.2'] },
    { variant: 'florida', versions: ['16.7.19', '16.6.1'] },
    { variant: 'undetected', versions: ['16.5.9', '16.4.4', '404.0.0'] },
  ],
  apps: [
    { package: 'com.tencent.mm', system: false },
    { package: 'com.ss.android.ugc.aweme', system: false },
    { package: 'com.bilibili.app', system: false },
    { package: 'org.telegram.messenger', system: false },
    { package: 'com.xingin.matrix', system: false },
    { package: 'com.spotify.music', system: false },
    { package: 'com.android.chrome', system: false },
    { package: 'com.zhiliaoapp.musically', system: false },
    { package: 'com.netease.cloudmusic', system: false },
    { package: 'com.android.systemui', system: true },
    { package: 'com.miui.home', system: true },
    { package: 'com.android.settings', system: true },
    { package: 'com.google.android.gms', system: true },
  ] as AppItem[],
  rules: {
    rules: [
      { app_name: 'com.tencent.mm', enabled: true, start_up_delay_ms: 800, child_gating_enabled: false, child_gating_mode: 'freeze', custom_gadget: '' },
      { app_name: 'com.ss.android.ugc.aweme', enabled: true, start_up_delay_ms: 0, child_gating_enabled: true, child_gating_mode: 'freeze', custom_gadget: '' },
      { app_name: 'org.telegram.messenger', enabled: true, start_up_delay_ms: 0, child_gating_enabled: false, child_gating_mode: 'freeze', custom_gadget: 'official_17.2.14_arm64.so' },
      { app_name: 'com.bilibili.app', enabled: false, start_up_delay_ms: 0, child_gating_enabled: false, child_gating_mode: 'freeze', custom_gadget: '' },
    ],
  } as Rules,
  settings: {
    server: {
      autostart: true,
      active: 'official_17.2.14_arm64',
      args: ['-l', '127.0.0.1:27042'],
      restart_on_crash: true,
    } as ServerSettings,
    gadget: { active: 'official_17.2.14_arm64.so' },
    download: { mirror: '', github_api: 'https://api.github.com' },
  },
  web: { running: true, enabled: true, port: 23333, token: 'm0cktok42' },
  tasks: new Map<string, MockTask>(),
  taskSeq: 0,
  /** 当前进行中的任务（ksu tasks/current 哨兵语义） */
  currentTaskId: null as string | null,
}

/* frida-server 约 25MB / 导入上传约 8MB 的演示量级 */
const MOCK_TOTAL = 25_480_000
const MOCK_UPLOAD = 8_200_000

/* ------------------------------------------------------------------ */
/* 视图构造                                                            */
/* ------------------------------------------------------------------ */

function srvStatus(): SrvStatus {
  return {
    running: db.running,
    pid: db.running ? db.pid : undefined,
    binary: db.running ? db.settings.server.active : '',
    uptime_sec: db.running ? Math.floor((Date.now() - db.startedAt) / 1000) : 0,
    note: db.note,
  }
}

function status(): Status {
  return {
    server: srvStatus(),
    servers: db.servers,
    gadgets: db.gadgets,
    rules: db.rules,
    web: db.web,
    settings: db.settings,
  }
}

/** 生成 manifest 完整元数据（file 是不透明 key，不解析文件名） */
function newBinary(variant: string, version: string, type: string): Binary {
  const suffix = type === 'gadget' ? '_arm64.so' : '_arm64'
  const file = `${variant}_${version}${suffix}`
  const list = type === 'gadget' ? db.gadgets : db.servers
  const found = list.find((b) => b.file === file)
  if (found) {
    found.missing = false
    found.added_at = new Date().toISOString()
    return found
  }
  const b = mkBinary({
    file,
    variant,
    version,
    elf_type: type === 'gadget' ? 'dyn' : 'exec',
    source: variant === 'official' ? `https://github.com/frida/frida/releases/tag/${version}` : `https://github.com/${variant}/frida/releases/tag/${version}`,
    size: 20_000_000 + Math.floor(Math.random() * 6_000_000),
    added_at: new Date().toISOString(),
  })
  list.push(b)
  return b
}

function isFailVersion(version: string): boolean {
  return version.includes('404') || version.toLowerCase().includes('fail')
}

function createTask(kind: MockTask['kind'], variant: string, version: string, type: string): string {
  const id = `t${Date.now().toString(36)}${++db.taskSeq}`
  db.tasks.set(id, { id, created: Date.now(), kind, variant, version, type, fail: isFailVersion(version), done: false })
  db.currentTaskId = id
  return id
}

/** 43 字符 base64url 风格伪随机 token（模拟 core.GenToken） */
function genMockToken(): string {
  const chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_'
  let s = ''
  for (let i = 0; i < 43; i++) s += chars[Math.floor(Math.random() * chars.length)]
  return s
}

/** 按经过时间推进 phase：resolve → download → decompress → install → done/error */
function taskView(id: string): Task {
  const t = db.tasks.get(id)
  if (!t) throw new Error('任务不存在：' + id)
  const el = (Date.now() - t.created) / 1000
  const label = `${t.variant} ${t.version}`

  let state = 'running'
  let phase = 'download'
  let detail = label
  let error = ''
  let bytesDone = 0
  let bytesTotal = 0 // <=0 表示未知

  if (t.kind === 'download') {
    if (t.fail && el > 1.2) {
      state = 'error'
      error = `下载失败：HTTP 404 Not Found（${label}）`
    } else if (el < 0.8) {
      phase = 'resolve'
      detail = `解析版本 ${label}`
    } else if (el < 2.8) {
      // 伪 % 逻辑换算为字节：0.8s–2.8s 线性推进至 ~25MB
      const ratio = Math.min(1, (el - 0.8) / 1.6)
      phase = 'download'
      detail = label
      bytesTotal = MOCK_TOTAL
      bytesDone = Math.floor(MOCK_TOTAL * ratio)
    } else if (el < 3.8) {
      phase = 'decompress'
      detail = `${label} · 解压中`
      // 解压无字节进度 → total 0 = 未知 → 前端 indeterminate
    } else if (el < 5) {
      phase = 'install'
      detail = `${label} · 安装到 manifest`
    } else {
      state = 'done'
      detail = label
      bytesDone = MOCK_TOTAL
      bytesTotal = MOCK_TOTAL
    }
  } else {
    // import：上传（带字节进度）→ 安装
    if (el < 1.5) {
      phase = 'download'
      detail = `上传中 ${t.variant}`
      bytesTotal = MOCK_UPLOAD
      bytesDone = Math.floor(MOCK_UPLOAD * Math.min(1, el / 1.2))
    } else if (el < 3) {
      phase = 'install'
      detail = `安装 ${t.variant} ${t.version}`
    } else {
      state = 'done'
      detail = label
      bytesDone = MOCK_UPLOAD
      bytesTotal = MOCK_UPLOAD
    }
  }

  // done 时一次性写入列表并清掉哨兵（模拟安装副作用）
  if (state === 'done' && !t.done) {
    t.done = true
    newBinary(t.variant, t.version, t.type)
    if (db.currentTaskId === id) db.currentTaskId = null
  }

  return {
    id,
    state,
    phase,
    detail,
    bytes_done: bytesDone,
    bytes_total: bytesTotal,
    error,
    created_at: new Date(t.created).toISOString(),
    updated_at: new Date().toISOString(),
  }
}

function pick<T>(arr: T[]): T {
  return arr[Math.floor(Math.random() * arr.length)]
}

function mockLogs(name: string, tail: number): { name: string; lines: string[] } {
  // web 服务未运行时模拟「文件不存在」空态
  if (name === 'web' && !db.web.running) return { name, lines: [] }

  const now = Date.now()
  const lines: string[] = []
  const ctl = ['utsusemi-ctl ready (module v0.3.0)', 'gadget apply: 4 rules -> gadget.json', 'server start: official_17.2.14_arm64', 'bin download done: florida 16.7.19 (22.8 MB)', 'watchdog: server alive (pid 4173)']
  const server = ['frida 17.2.14 arm64 listening on 127.0.0.1:27042', 'spawned: com.tencent.mm (pid 8321)', 'resume: com.tencent.mm', 'agent loaded: frida-agent-64.so', 'unhandled exception in script: TypeError (ignored)']
  const web = ['[GIN] 200 | 2.1ms | GET /api/status', '[GIN] 200 | 0.8ms | GET /api/apps', '[GIN] 401 | 0.1ms | GET /api/bin', '[GIN] 200 | 15.3ms | POST /api/bin/download', 'token auth ok (bearer m0ck****42)']

  for (let i = tail; i > 0; i--) {
    const ts = new Date(now - i * 1500).toISOString()
    const pool = name === 'ctl' ? ctl : name === 'server' ? server : web
    lines.push(`${ts} [${name}] ${pick(pool)}`)
  }
  return { name, lines }
}

/* ------------------------------------------------------------------ */
/* 请求路由                                                            */
/* ------------------------------------------------------------------ */

export async function mockRequest<T>(rest: RestSpec | null, ctl: CtlSpec | null): Promise<T> {
  await sleep(120 + Math.random() * 280)
  const key = ctl?.sub ?? (rest ? `${rest.method} ${rest.path}` : '?')

  switch (key) {
    /* ---------------- 总览 / server ---------------- */
    case 'status':
    case 'GET /api/status':
      return status() as T
    case 'server/start':
    case 'POST /api/server/start': {
      db.running = true
      db.startedAt = Date.now()
      db.note = ''
      db.pid = Math.floor(2000 + Math.random() * 6000)
      return srvStatus() as T
    }
    case 'server/stop':
    case 'POST /api/server/stop': {
      db.running = false
      db.note = ''
      return srvStatus() as T
    }
    case 'server/restart':
    case 'POST /api/server/restart': {
      db.running = true
      db.startedAt = Date.now()
      db.pid = Math.floor(2000 + Math.random() * 6000)
      return srvStatus() as T
    }
    case 'server/set':
    case 'PUT /api/server': {
      const payload = (ctl?.payload ?? rest?.body) as ServerSettings
      db.settings.server = JSON.parse(JSON.stringify(payload))
      return { saved: true } as T
    }

    /* ---------------- 应用注入 ---------------- */
    case 'apps':
    case 'GET /api/apps':
      return db.apps as T
    case 'gadget':
    case 'GET /api/gadget':
      return db.rules as T
    case 'gadget/set':
    case 'PUT /api/gadget': {
      const payload = (ctl?.payload ?? rest?.body) as Rules
      db.rules = JSON.parse(JSON.stringify(payload))
      return { applied: db.rules.rules.filter((r) => r.enabled).length } as T
    }

    /* ---------------- 二进制管理 ---------------- */
    case 'bin/list':
    case 'GET /api/bin':
      return { servers: db.servers, gadgets: db.gadgets } as T
    case 'bin/sources':
    case 'GET /api/bin/sources':
      return db.sources as T
    case 'bin/use': {
      const type = String(ctl?.flags?.type ?? '')
      const file = String(ctl?.flags?.file ?? '')
      if (type === 'gadget') db.settings.gadget.active = file
      else db.settings.server.active = file
      return { active: file } as T
    }
    case 'bin/cleanup':
    case 'POST /api/bin/cleanup': {
      const removed: string[] = []
      db.servers = db.servers.filter((b) => (b.file === db.settings.server.active ? true : (removed.push(b.file), false)))
      db.gadgets = db.gadgets.filter((b) => (b.file === db.settings.gadget.active ? true : (removed.push(b.file), false)))
      return { removed } as T
    }
    case 'bin/download':
    case 'POST /api/bin/download': {
      const body = (rest?.body ?? {}) as { variant?: string; type?: string; version?: string }
      const variant = String(ctl?.flags?.variant ?? body.variant ?? 'official')
      const type = String(ctl?.flags?.type ?? body.type ?? 'server')
      const version = String(ctl?.flags?.version ?? body.version ?? db.sources.find((s) => s.variant === variant)?.versions[0] ?? '17.2.14')
      if (isFailVersion(version)) {
        // REST：先建任务后失败；ksu：同步直接失败
        if (key.startsWith('bin/')) await sleep(1600)
        throw new Error(`下载失败：HTTP 404 Not Found（${variant} ${version}）`)
      }
      if (key.startsWith('bin/')) {
        // ksu 同步模式：期间建哨兵任务供 tasks/current 轮询字节进度，结束后返回 Binary
        const id = createTask('download', variant, version, type)
        await sleep(2200)
        const t = db.tasks.get(id)
        if (t) t.done = true
        db.currentTaskId = null
        return newBinary(variant, version, type) as T
      }
      return { task_id: createTask('download', variant, version, type) } as T
    }
    case 'bin/import': {
      if (key.startsWith('bin/')) {
        // ksu 同步导入本地路径
        await sleep(1400)
        return newBinary('custom', String(ctl?.flags?.version ?? '1.0.0'), String(ctl?.flags?.type ?? 'server')) as T
      }
      const form = rest?.form
      const type = String(form?.get('type') ?? 'server')
      const version = String(form?.get('version') ?? '1.0.0')
      const f = form?.get('file')
      const fname = f instanceof File ? f.name : 'custom.bin'
      return { task_id: createTask('import', fname.replace(/\.[^.]*$/, '').replace(/[^\w.-]/g, '_'), version, type) } as T
    }
    default: {
      /* 形态带参数的端点单独匹配 */
      if (key.startsWith('tasks/')) {
        // ksu tasks/current：读哨兵，无进行中任务时 ok:false（契约 §4.6）
        if (key === 'tasks/current') {
          if (!db.currentTaskId) throw new Error('no current task')
          return taskView(db.currentTaskId) as T
        }
        return taskView(key.slice(6)) as T
      }
      // REST 任务进度：GET /api/tasks/:id
      const taskRest = key.match(/^GET \/api\/tasks\/(.+)$/)
      if (taskRest) return taskView(decodeURIComponent(taskRest[1])) as T
      const del = key.match(/^DELETE \/api\/bin\/(.+)$/)
      if (del && rest) {
        const file = decodeURIComponent(del[1])
        const type = new URL(rest.path, 'http://x').searchParams.get('type') ?? 'server'
        const list = type === 'gadget' ? db.gadgets : db.servers
        const idx = list.findIndex((b) => b.file === file)
        if (idx >= 0) list.splice(idx, 1)
        return { removed: file } as T
      }
      if (key === 'logs' || key.startsWith('GET /api/logs')) {
        const u = new URL(rest?.path ?? '/api/logs?name=ctl&tail=200', 'http://x')
        const name = String(ctl?.flags?.name ?? u.searchParams.get('name') ?? 'ctl')
        const tail = Number(ctl?.flags?.tail ?? u.searchParams.get('tail') ?? 200)
        return mockLogs(name, tail) as T
      }
      if (key === 'web/info' || key === 'GET /api/web/info') return { ...db.web } as T
      if (key === 'POST /api/web/stop') {
        db.web.running = false
        return { stopping: true } as T
      }
      if (key === 'web/token' || key === 'PUT /api/web/token') {
        // 契约 §4.8：generate 或自定义 token；空/{} payload = 读取当前
        const payload = (ctl?.payload ?? rest?.body ?? {}) as { token?: string; generate?: boolean }
        let target = db.web.token
        if (payload.generate) {
          target = genMockToken()
        } else if (typeof payload.token === 'string' && payload.token.trim() !== '') {
          const trimmed = payload.token.trim()
          // 与后端 validateToken 同规则：trim 后 8–128 位、ASCII 可打印（0x21–0x7E）无空白
          if (trimmed.length < 8 || trimmed.length > 128 || !/^[\x21-\x7E]+$/.test(trimmed)) {
            throw new Error('token 校验失败：长度需 8–128 字符，仅限 ASCII 可打印字符且不含空白')
          }
          target = trimmed
        }
        db.web.token = target
        // restart_required = web 服务正在运行（webAlive）
        return { token: target, restart_required: db.web.running } as T
      }
      throw new Error('mock 未实现的调用：' + key)
    }
  }
}

/** 非 api 信封的原生命令 mock（utsusemi-ctl web start/stop） */
export async function mockCtlRaw(args: string): Promise<string> {
  await sleep(300)
  if (args.startsWith('web start')) {
    const port = Number(args.match(/--port\s+(\d+)/)?.[1] ?? 0)
    if (port) db.web.port = port
    db.web.running = true
    db.web.enabled = true
    return `web server started on :${db.web.port}`
  }
  if (args.trim() === 'web stop') {
    db.web.running = false
    return 'web server stopped'
  }
  throw new Error('mock 未实现的原生命令：' + args)
}
