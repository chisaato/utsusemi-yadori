/**
 * 统一业务/传输错误。
 * 契约 §5：无错误码体系，message 直接面向用户展示。
 */
export class ApiError extends Error {
  /** HTTP 状态码（仅 REST 401 等场景有意义，0 = 非 HTTP 来源） */
  readonly code: number
  /** true = 令牌失效，需要从带 ?token= 的入口链接重新进入 */
  readonly isAuth: boolean

  constructor(message: string, code = 0, isAuth = false) {
    super(message)
    this.name = 'ApiError'
    this.code = code
    this.isAuth = isAuth
  }
}
