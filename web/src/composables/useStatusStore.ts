import { ref, shallowRef } from 'vue'
import { getStatus } from '../api'
import type { Status } from '../api/types'

/**
 * 总览状态单例：Status 一次拉全（server/servers/gadgets/rules/web/settings），
 * 各页面（总览/注入/二进制/远程）共享，避免重复请求。
 */
const status = shallowRef<Status | null>(null)
const loading = ref(false)
const error = ref('')
const fetchedAt = ref(0)

async function refresh(): Promise<boolean> {
  loading.value = true
  error.value = ''
  try {
    status.value = await getStatus()
    fetchedAt.value = Date.now()
    return true
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
    return false
  } finally {
    loading.value = false
  }
}

export function useStatusStore() {
  return { status, loading, error, fetchedAt, refresh }
}
