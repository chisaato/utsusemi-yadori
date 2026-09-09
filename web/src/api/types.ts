/** 契约 §1 统一信封 */
export interface Envelope<T> {
  ok: boolean
  data?: T
  error?: string
}

/** 契约 §4.5 Binary：manifest 全量元数据 */
export interface Binary {
  /** 不透明 key，禁止解析还原元数据 */
  file: string
  /** official | florida | undetected | custom */
  variant: string
  version: string
  /** arm64 | arm | x86_64 | x86 | unknown */
  arch: string
  /** exec | dyn（server=exec / gadget=dyn） */
  elf_type: string
  source: string
  sha256: string
  size: number
  added_at: string
  /** true = manifest 有记录但文件丢失，UI 须标红提示重新导入 */
  missing: boolean
}

/** 契约 §4.1 内 server 字段 */
export interface BinUseResult {
  active: string
  stopped_previous?: boolean
}

export interface SrvStatus {
  running: boolean
  pid?: number
  binary?: string
  uptime_sec?: number
  /** 非空 = 异常原因 */
  note?: string
}

/** 契约 §4.2 ServerSettings（PUT /api/server 全量替换） */
export interface ServerSettings {
  autostart: boolean
  active: string
  args: string[]
  restart_on_crash: boolean
}

/** 契约 §4.3 App */
export interface AppItem {
  package: string
  system: boolean
}

/** 契约 §4.4 Rule */
export interface Rule {
  app_name: string
  enabled: boolean
  start_up_delay_ms: number
  child_gating_enabled: boolean
  /** freeze | wait | kill */
  child_gating_mode: string
  /** 空 = 用全局激活 gadget */
  custom_gadget: string
}

export interface Rules {
  rules: Rule[]
}

/** 契约 §4.1 Status：总览首屏一次拉全 */
export interface Status {
  server: SrvStatus
  servers: Binary[]
  gadgets: Binary[]
  rules: Rules
  web: {
    enabled: boolean
    port: number
    token: string
  }
  settings: {
    server: ServerSettings
    gadget: { active: string }
    download: { mirror: string; github_api: string }
  }
}

/** 契约 §3 可用版本 */
export interface BinSource {
  variant: string
  versions: string[]
}

/** 契约 §4.6 Task：下载/导入进度轮询 */
export interface Task {
  id: string
  /** running | done | error */
  state: string
  /** resolve | download | decompress | install（后端 omitempty，可能缺省） */
  phase?: string
  detail?: string
  /** 已完成字节；bytes_total <= 0 表示未知总大小 */
  bytes_done: number
  bytes_total: number
  error?: string
  created_at: string
  updated_at: string
}

/** 契约 §4.7 WebInfo */
export interface WebInfo {
  running: boolean
  enabled: boolean
  port: number
  token: string
}

/** 契约 §4.8 WebToken 返回；restart_required = web 服务运行中，需重启生效 */
export interface WebTokenResult {
  token: string
  restart_required: boolean
}

/** ADB 实时状态 */
export interface AdbCurrentStatus {
  adbd_running: boolean
  usb_enabled: boolean
  tcpip_enabled: boolean
  tcpip_port: number
  ips: string[]
}

/** ADB 持久化设置 */
export interface AdbSettings {
  usb_enabled: boolean
  tcpip_enabled: boolean
  port: number
  apply_on_boot: boolean
}

/** ADB 完整状态响应 */
export interface AdbStatus {
  current: AdbCurrentStatus
  settings: AdbSettings
}

