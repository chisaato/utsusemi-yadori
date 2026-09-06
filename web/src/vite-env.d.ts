/// <reference types="vite/client" />

// VITE_LIVE=1 时 dev 直连真实后端（不走 mock）
interface ImportMetaEnv {
  readonly VITE_LIVE?: string
}
