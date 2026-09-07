/**
 * 契约 §3 端点映射表的强类型封装。
 * 每个函数同时给出 REST 与 ctl 两种形态，传输选择由 client.request 决定。
 */
import { request } from './client'
import type { AppItem, Binary, BinSource, Rules, ServerSettings, Status, SrvStatus, Task, WebInfo, WebTokenResult } from './types'

/* ---------------- 总览 / server ---------------- */

export const getStatus = () =>
  request<Status>({ method: 'GET', path: '/api/status' }, { sub: 'status' })

export const serverStart = () =>
  request<SrvStatus>({ method: 'POST', path: '/api/server/start' }, { sub: 'server/start' })

export const serverStop = () =>
  request<SrvStatus>({ method: 'POST', path: '/api/server/stop' }, { sub: 'server/stop' })

export const serverRestart = () =>
  request<SrvStatus>({ method: 'POST', path: '/api/server/restart' }, { sub: 'server/restart' })

export const serverSet = (s: ServerSettings) =>
  request<{ saved: boolean }>({ method: 'PUT', path: '/api/server', body: s }, { sub: 'server/set', payload: s })

/* ---------------- 应用注入 ---------------- */

export const getApps = () =>
  request<AppItem[]>({ method: 'GET', path: '/api/apps' }, { sub: 'apps' })

export const getGadget = () =>
  request<Rules>({ method: 'GET', path: '/api/gadget' }, { sub: 'gadget' })

/** 写入后自动 apply 重建 gadget.json；apply_error 非空表示保存成功但 apply 失败 */
export const setGadget = (r: Rules) =>
  request<{ applied: number; apply_error?: string }>(
    { method: 'PUT', path: '/api/gadget', body: r },
    { sub: 'gadget/set', payload: r },
  )

/* ---------------- 二进制管理 ---------------- */

export const binList = () =>
  request<{ servers: Binary[]; gadgets: Binary[] }>({ method: 'GET', path: '/api/bin' }, { sub: 'bin/list' })

export const binSources = () =>
  request<BinSource[]>({ method: 'GET', path: '/api/bin/sources' }, { sub: 'bin/sources' })

/** REST 返回 task_id 异步轮询；ksu 同步返回 Binary */
export type DownloadResult = { task_id: string } | Binary

export const binDownload = (p: { variant: string; type: string; version?: string }) =>
  request<DownloadResult>(
    { method: 'POST', path: '/api/bin/download', body: p },
    { sub: 'bin/download', flags: { variant: p.variant, type: p.type, version: p.version } },
  )

/** 文件上传导入仅 REST（multipart: file + type + version） */
export const binImportRest = (type: string, version: string, file: File) => {
  const form = new FormData()
  form.append('file', file)
  form.append('type', type)
  form.append('version', version)
  return request<{ task_id: string }>({ method: 'POST', path: '/api/bin/import', form }, null)
}

/** ksu 模式同步导入本地路径文件 */
export const binImportKsu = (type: string, file: string, version: string) =>
  request<Binary>(null, { sub: 'bin/import', flags: { type, file, version } })

export const binRemove = (type: string, file: string) =>
  request<{ removed: string }>(
    { method: 'DELETE', path: `/api/bin/${encodeURIComponent(file)}?type=${encodeURIComponent(type)}` },
    { sub: 'bin/remove', flags: { type, file } },
  )

export const binUse = (type: string, file: string) =>
  request<{ active: string }>(
    { method: 'POST', path: '/api/bin/use', body: { type, file } },
    { sub: 'bin/use', flags: { type, file } },
  )

export const binCleanup = () =>
  request<{ removed: string[] }>({ method: 'POST', path: '/api/bin/cleanup' }, { sub: 'bin/cleanup' })

/**
 * 任务进度：REST 按 id 轮询；ksu 形态固定 tasks/current（哨兵文件，无 by-id 查询，契约 §4.6）。
 */
export const getTask = (id: string) =>
  request<Task>(
    { method: 'GET', path: `/api/tasks/${encodeURIComponent(id)}` },
    { sub: 'tasks/current' },
  )

/** ksu 当前任务（哨兵文件）；无进行中任务时 ok:false 抛错。仅 ksu 模式使用。 */
export const getTaskCurrent = () => request<Task>(null, { sub: 'tasks/current' })

/* ---------------- 日志 / 远程协同 ---------------- */

export const getLogs = (name: string, tail: number) =>
  request<{ name: string; lines: string[] }>(
    { method: 'GET', path: `/api/logs?name=${encodeURIComponent(name)}&tail=${tail}` },
    { sub: 'logs', flags: { name, tail } },
  )

export const webInfo = () =>
  request<WebInfo>({ method: 'GET', path: '/api/web/info' }, { sub: 'web/info' })

/** Web Token 管理（契约 §4.8）：generate 或传自定义 token；空 payload = 读取当前 */
export const setWebToken = (payload: { token?: string; generate?: boolean }) =>
  request<WebTokenResult>(
    { method: 'PUT', path: '/api/web/token', body: payload },
    { sub: 'web/token', payload },
  )

/** 远程关停仅 REST；ksu 模式的启停用 client.execCtlRaw 走原生命令 */
export const webStop = () =>
  request<{ stopping: boolean }>({ method: 'POST', path: '/api/web/stop' }, null)
