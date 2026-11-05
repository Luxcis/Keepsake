import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'
import { fileURLToPath, URL } from 'node:url'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    // 开发态由 Vite 代理 /api → 本地 Go API（避免 CORS）；生产由 Nginx 提供同源。
    // 不要开启 changeOrigin：Go 端 CSRF 校验比较 Origin 与 Host，改写 Host 为 :8080
    // 会让浏览器同源请求（Origin: :5173）被误判为跨站；与 Nginx 保留 $http_host 同理。
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: false,
      },
    },
  },
  build: {
    sourcemap: false,
  },
})
