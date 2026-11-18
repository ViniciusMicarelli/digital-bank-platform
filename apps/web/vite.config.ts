/// <reference types="vitest/config" />
import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'
import { fileURLToPath } from 'node:url'

// The API gateway runs on :8000 (make up). Proxying keeps the app and the
// API on one origin: no CORS, and cookies/headers behave as in production.
const gateway = process.env.GATEWAY_URL ?? 'http://127.0.0.1:8000'

export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { '@bank/core': fileURLToPath(new URL('../../packages/ts-core/src/index.ts', import.meta.url)) } },
  server: {
    fs: { allow: ['../..'] },
    port: 5173,
    proxy: {
      '/v1': { target: gateway, changeOrigin: true },
      '/pix/qr': { target: gateway, changeOrigin: true },
    },
  },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    include: ['src/**/*.test.ts', '../../packages/ts-core/src/**/*.test.ts'],
  },
})
