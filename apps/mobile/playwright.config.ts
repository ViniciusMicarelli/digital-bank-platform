import { defineConfig, devices } from '@playwright/test'

// The mobile app built for the web (react-native-web), on a phone-sized
// viewport, against the real stack (`make up`; the gateway allows this origin
// through CORS_ALLOWED_ORIGINS). Build first: EXPO_PUBLIC_API_URL=http://localhost:8000 npm run export:web
export default defineConfig({
  testDir: './e2e',
  timeout: 90_000,
  expect: { timeout: 15_000 },
  fullyParallel: true,
  reporter: process.env.CI ? 'github' : 'list',
  use: { ...devices['iPhone 15'], browserName: 'chromium', colorScheme: process.env.COLOR_SCHEME === 'dark' ? 'dark' : 'light', baseURL: 'http://localhost:8082', locale: 'pt-BR', trace: 'retain-on-failure' },
  webServer: { command: 'npx serve -s dist -l 8082', url: 'http://localhost:8082', reuseExistingServer: true },
})
