import { defineConfig, devices } from '@playwright/test';

const port = 4177;

export default defineConfig({
	testDir: 'tests',
	// Live components count everyone online, so tests run one at a time.
	fullyParallel: false,
	workers: 1,
	forbidOnly: !!process.env.CI,
	retries: process.env.CI ? 1 : 0,
	reporter: process.env.CI ? [['github'], ['list']] : 'list',
	use: { baseURL: `http://localhost:${port}`, trace: 'retain-on-failure' },
	projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
	webServer: {
		command: `go tool tern build -o build/e2e && PORT=${port} ./build/e2e`,
		url: `http://localhost:${port}/`,
		reuseExistingServer: !process.env.CI,
		timeout: 240_000
	}
});
