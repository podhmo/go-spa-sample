import react from '@vitejs/plugin-react'
import { playwright } from '@vitest/browser-playwright'
import { defineConfig } from 'vitest/config'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      // Fake Go API server (cmd/fakeapi) — same-origin /api, no CORS needed.
      '/api': 'http://127.0.0.1:8080',
    },
  },
  test: {
    browser: {
      enabled: true,
      // Use the system Chrome instead of downloading Playwright's chromium.
      provider: playwright({ launchOptions: { channel: 'chrome' } }),
      instances: [{ browser: 'chromium' }],
    },
  },
})
