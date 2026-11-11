import { defineConfig, devices } from '@playwright/test';

// Browser tests: npm run test:e2e. The first run needs the browser:
// npx playwright install chromium
export default defineConfig({
	testDir: 'tests',
	use: { baseURL: 'http://localhost:4173' },
	webServer: {
		// The production build, as users get it.
		command: 'go tool tern build -o build/e2e && ./build/e2e',
		env: { PORT: '4173' },
		url: 'http://localhost:4173',
		reuseExistingServer: !process.env.CI,
		timeout: 180_000
	},
	projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }]
});
