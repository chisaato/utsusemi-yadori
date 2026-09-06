import { ref } from 'vue'

/** 响应式 matchMedia（SSR 无关，本项目纯客户端） */
export function useMedia(query: string) {
  const m = window.matchMedia(query)
  const matches = ref(m.matches)
  m.addEventListener('change', (e) => {
    matches.value = e.matches
  })
  return matches
}
