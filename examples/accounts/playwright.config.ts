import { defineConfig, devices } from '@playwright/test';

const port = 4182;

export default defineConfig({
	testDir: 'tests',
	fullyParallel: true,
	forbidOnly: !!process.env.CI,
	retries: process.env.CI ? 1 : 0,
	reporter: process.env.CI ? [['github'], ['list']] : 'list',
	use: { baseURL: `http://localhost:${port}`, trace: 'retain-on-failure' },
	projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
	webServer: {
		command: `go tool tern build -o build/e2e && rm -f build/e2e.db* && ACCOUNTS_NO_RATE_LIMITS=1 DATABASE=build/e2e.db PORT=${port} ./build/e2e`,
		url: `http://localhost:${port}/`,
		reuseExistingServer: !process.env.CI,
		timeout: 180_000
	}
});
