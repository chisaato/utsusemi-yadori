import { computed, ref, watchEffect } from 'vue'

export type ThemePref = 'auto' | 'dark' | 'light'

const KEY = 'utsusemi_theme'

function readPref(): ThemePref {
  const v = localStorage.getItem(KEY)
  return v === 'dark' || v === 'light' || v === 'auto' ? v : 'auto'
}

/** 模块级单例：全 app 共享同一份主题状态 */
const pref = ref<ThemePref>(readPref())

// 跟随系统：auto 模式下监听 prefers-color-scheme 变化
const systemDark = ref<boolean>(window.matchMedia('(prefers-color-scheme: dark)').matches)
window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
  systemDark.value = e.matches
})

const isDark = computed(() => (pref.value === 'auto' ? systemDark.value : pref.value === 'dark'))

// html.dark 供自定义 CSS 令牌切换；Naive 侧由 App.vue 读取 isDark
watchEffect(() => {
  document.documentElement.classList.toggle('dark', isDark.value)
})

function setPref(p: ThemePref) {
  pref.value = p
  // 仅手动选择时持久化（首次读取的默认值不写入）
  localStorage.setItem(KEY, p)
}

export function useTheme() {
  /** auto → dark → light → auto 循环 */
  const cycle = () => {
    const next: ThemePref = pref.value === 'auto' ? 'dark' : pref.value === 'dark' ? 'light' : 'auto'
    setPref(next)
  }
  return { pref, isDark, cycle, setPref }
}
