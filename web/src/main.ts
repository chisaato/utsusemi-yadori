import { createApp } from 'vue'
import App from './App.vue'
import { router } from './router'
import { captureToken } from './api/client'
import './styles/main.css'
import '@fontsource/jetbrains-mono/400.css'
import '@fontsource/jetbrains-mono/500.css'
import '@fontsource/jetbrains-mono/700.css'

// 入口 token 捕获必须在挂载前完成（契约 §2）
captureToken()

createApp(App).use(router).mount('#app')
