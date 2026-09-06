import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// 产物拷贝至 template/webroot/（KSU WebUI）并 embed 进 Go 二进制（PC 模式），相对路径必须可移植
export default defineConfig({
  base: './',
  plugins: [vue()],
  build: {
    outDir: 'dist',
    target: 'es2020',
  },
  server: {
    port: 5188,
    host: '127.0.0.1',
  },
})
