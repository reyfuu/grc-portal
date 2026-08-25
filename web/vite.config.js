import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  // Build straight into the Go embed directory so `go build` ships one binary.
  build: { outDir: '../internal/web/dist', emptyOutDir: true },
  // Dev: proxy API calls to the Go server on :8081 (8080 taken by local Apache).
  server: { proxy: { '/api': 'http://localhost:8081' } },
})
