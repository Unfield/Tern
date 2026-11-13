import { defineConfig, devices } from '@playwright/test';

const port = 4173;

export default defineConfig({
	testDir: 'tests',
	forbidOnly: !!process.env.CI,
	retries: process.env.CI ? 1 : 0,
	reporter: process.env.CI ? [['github'], ['list']] : 'list',
	use: {
		baseURL: `http://localhost:${port}`,
		trace: 'retain-on-failure'
	},
	projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
	webServer: {
		// The production build, on an empty database.
		command: `rm -f build/e2e.db* && go tool tern build -o build/e2e && DATABASE=build/e2e.db PORT=${port} ./build/e2e`,
		url: `http://localhost:${port}/login`,
		reuseExistingServer: !process.env.CI,
		timeout: 180_000
	}
});
