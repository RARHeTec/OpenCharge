import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// The admin API runs on :8081; proxy /api there so the browser sees same-origin
// requests and we avoid CORS entirely.
export default defineConfig({
  plugins: [vue()],
  server: {
    host: true,
    port: 5173,
    proxy: {
      '/api': { target: 'http://127.0.0.1:8081', changeOrigin: true },
    },
  },
})
