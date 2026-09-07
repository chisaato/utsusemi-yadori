import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import basicSsl from '@vitejs/plugin-basic-ssl'

// 产物拷贝至 template/webroot/（KSU WebUI）并 embed 进 Go 二进制（PC 模式），相对路径必须可移植
//
// dev 模式（环境变量驱动）：
//   bun run dev          本机 mock 调试（无 VITE_LIVE 时 request() 走 mockRequest）
//   bun run dev:remote   PC 浏览器渲染 + API 代理到设备上的 utsusemi-ctl web serve（REST）：
//                        VITE_API_TARGET=http://<设备IP>:<port> bun run dev:remote
//   bun run dev:https    设备 WebView/WebUI-X 直接加载本 dev server（window.ksu 有效，
//                        走 ksu.exec 直调设备 ctl；WebView 拒绝明文，必须 HTTPS，自签证书）：
//                        bun run dev:https（监听 0.0.0.0，平板访问 https://<PC IP>:5188）
const httpsDev = process.env.DEV_HTTPS === '1'
const apiTarget = process.env.VITE_API_TARGET

export default defineConfig({
  base: './',
  plugins: [vue(), ...(httpsDev ? [basicSsl()] : [])],
  build: {
    outDir: 'dist',
    target: 'es2020',
  },
  server: {
    port: 5188,
    host: httpsDev ? '0.0.0.0' : '127.0.0.1',
    // HTTPS 证书由 basicSsl() 插件注入（Vite 8 的 https 需完整 ServerOptions，不能传 true）
    // 模式 A：REST 透传（默认仅 /api 前缀；mock 模式不 fetch，代理空置无害）
    ...(apiTarget ? { proxy: { '/api': { target: apiTarget, changeOrigin: true } } } : {}),
  },
})
