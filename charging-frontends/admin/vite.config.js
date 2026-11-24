import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// The admin API runs on :8081 behind API-key auth. The dev proxy injects the
// key so the browser stays same-origin (no CORS) and no key lands in the bundle.
// Set ADMIN_API_KEY to match the gateway's -admin-key entry.
const adminKey = process.env.ADMIN_API_KEY || 'key-t1'

export default defineConfig({
  plugins: [vue()],
  server: {
    host: true,
    port: 5173,
    proxy: {
      '/api': {
        target: process.env.ADMIN_API_BASE || 'http://127.0.0.1:8081',
        changeOrigin: true,
        headers: { 'X-API-Key': adminKey },
      },
    },
  },
})
