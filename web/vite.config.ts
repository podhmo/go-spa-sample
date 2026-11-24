import react from '@vitejs/plugin-react'
import { playwright } from '@vitest/browser-playwright'
import { defineConfig } from 'vitest/config'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      // Go API server (cmd/zatudan, ported zatudan API) — same-origin /api,
      // no CORS needed. The API is Bearer-token fail-closed, so the dev
      // proxy injects the credential the server was started with
      // (`-api-token dev-token`). Dev-only shortcut, not for production.
      '/api': {
        target: 'http://127.0.0.1:8080',
        configure: (proxy) => {
          proxy.on('proxyReq', (proxyReq) => {
            proxyReq.setHeader('authorization', 'Bearer dev-token')
          })
        },
      },
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
