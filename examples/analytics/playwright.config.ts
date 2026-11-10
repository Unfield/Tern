import { defineConfig, devices } from '@playwright/test';

const port = 4181;

export default defineConfig({
	testDir: 'tests',
	// The tests share one database and build on each other: the owner first.
	fullyParallel: false,
	workers: 1,
	forbidOnly: !!process.env.CI,
	retries: 0,
	reporter: process.env.CI ? [['github'], ['list']] : 'list',
	timeout: 60_000,
	use: { baseURL: `http://localhost:${port}`, trace: 'retain-on-failure' },
	projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
	webServer: {
		command: `go tool tern build -o build/e2e && rm -f build/e2e.db* && DEMO=1 DATABASE_PATH=build/e2e.db PORT=${port} ./build/e2e`,
		url: `http://localhost:${port}/healthz`,
		reuseExistingServer: !process.env.CI,
		timeout: 180_000
	}
});
