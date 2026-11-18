import { defineConfig, devices } from '@playwright/test'

// Browser tests against the real stack: `make up`, then `npm run e2e`
// (starts the Vite dev server, which proxies /v1 to the gateway), or
// BASE_URL=http://127.0.0.1:8081 to test the production image.
const baseURL = process.env.BASE_URL ?? 'http://localhost:5173'

export default defineConfig({
  testDir: './e2e',
  timeout: 60_000,
  expect: { timeout: 15_000 },
  fullyParallel: true,
  reporter: process.env.CI ? 'github' : 'list',
  use: { baseURL, colorScheme: process.env.COLOR_SCHEME === 'dark' ? 'dark' : 'light', locale: 'pt-BR', timezoneId: 'America/Sao_Paulo', trace: 'retain-on-failure' },
  projects: [
    { name: 'desktop', use: { ...devices['Desktop Chrome'] }, testIgnore: /mobile/ },
    { name: 'mobile', use: { ...devices['Pixel 7'] }, testMatch: /mobile/ },
  ],
  webServer: process.env.BASE_URL ? undefined : { command: 'npx vite --port 5173 --strictPort', url: baseURL, reuseExistingServer: true },
})
