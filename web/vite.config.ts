import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// Build output goes straight into the Go embed directory so a single
// `go build` produces a self-contained binary with the UI baked in.
export default defineConfig({
  plugins: [vue()],
  build: {
    outDir: '../internal/web/dist',
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    // During `npm run dev`, proxy API calls to the Go backend on :8000.
    proxy: {
      '/api': 'http://localhost:8000',
    },
  },
})
