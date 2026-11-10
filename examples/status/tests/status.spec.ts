import { expect, test, type Page } from '@playwright/test';
import { smtpSink } from './smtp.js';

// The tests run in order against one fresh database (see playwright.config.ts).
test.describe.configure({ mode: 'serial' });

const admin = { username: 'admin', password: 'correct horse battery' };
const target = '/api/test/target';

/** Collects console errors, page errors and hydration warnings. */
function watchConsole(page: Page) {
	const problems: string[] = [];
	page.on('console', (msg) => {
		if (msg.type() === 'error' || (msg.type() === 'warning' && /hydrat/i.test(msg.text()))) problems.push(msg.text());
	});
	page.on('pageerror', (err) => problems.push(err.message));
	return problems;
}

async function signIn(page: Page) {
	await page.goto('/login');
	await page.getByLabel('Username').fill(admin.username);
	await page.getByLabel('Password').fill(admin.password);
	await page.getByRole('button', { name: 'Sign in' }).click();
	await expect(page).toHaveURL(/\/admin$/);
}

async function setTarget(page: Page, code: number) {
	const res = await page.request.post(`${target}?code=${code}`);
	expect(res.ok()).toBe(true);
}

async function mark(page: Page) {
	await page.evaluate(() => ((window as any).__marker = true));
}

async function expectSameWindow(page: Page) {
	expect(await page.evaluate(() => (window as any).__marker)).toBe(true);
}

test('first run asks for an administrator account', async ({ page }) => {
	const problems = watchConsole(page);
	await page.goto('/admin');
	await expect(page).toHaveURL(/\/setup$/);

	await page.getByLabel('Username').fill(admin.username);
	await page.getByLabel('Password', { exact: true }).fill('short');
	await page.getByLabel('Confirm password').fill('short');
	await page.getByRole('button', { name: 'Create account' }).click();
	await expect(page.getByText('Use at least 10 characters.')).toBeVisible();

	await page.getByLabel('Password', { exact: true }).fill(admin.password);
	await page.getByLabel('Confirm password').fill(admin.password);
	await page.getByRole('button', { name: 'Create account' }).click();
	await expect(page).toHaveURL(/\/admin$/);
	await expect(page.getByText('No monitors yet')).toBeVisible();

	// Setup only works once.
	await page.goto('/setup');
	await expect(page).toHaveURL(/\/admin$/);
	expect(problems).toEqual([]);
});

test('signing in checks the password', async ({ page }) => {
	await page.goto('/login');
	await page.getByLabel('Username').fill(admin.username);
	await page.getByLabel('Password').fill('wrong password');
	await page.getByRole('button', { name: 'Sign in' }).click();
	await expect(page.getByRole('alert')).toHaveText('Wrong username or password.');
	await signIn(page);

	// Signed out, the admin pages send you back to the login page.
	await page.getByRole('button', { name: /Sign out/ }).click();
	await expect(page).toHaveURL(/\/$/);
	await page.goto('/admin/incidents');
	await expect(page).toHaveURL(/\/login\?next=%2Fadmin%2Fincidents|\/login\?next=\/admin\/incidents/);
});

test('monitors are validated, created and checked live', async ({ page }) => {
	const problems = watchConsole(page);
	await setTarget(page, 200);
	await signIn(page);
	await page.getByRole('link', { name: 'New monitor' }).click();

	await page.getByLabel('Name', { exact: true }).fill('API');
	await page.getByLabel('URL').fill('not a url');
	await page.getByRole('button', { name: 'Create monitor' }).click();
	await expect(page.getByText('Enter an http:// or https:// URL.')).toBeVisible();
	await expect(page.getByLabel('Name', { exact: true })).toHaveValue('API'); // the form keeps what was typed

	await page.getByLabel('URL').fill(`http://localhost:4176${target}`);
	await page.getByLabel('Interval (seconds)').fill('5');
	await page.getByLabel('Retries before down').fill('0');
	await page.getByLabel('Keyword').fill('target says');
	await page.getByLabel('Group').fill('Backend');
	await page.getByRole('button', { name: 'Create monitor' }).click();

	await expect(page).toHaveURL(/\/admin\/monitors\/\d+$/);
	await expect(page.getByText('API is being checked every 5 seconds.')).toBeVisible();
	await expect(page.getByTestId('detail-status')).toHaveText('Up', { timeout: 15_000 });

	await page.goto('/admin');
	const row = page.getByTestId('monitor-row').filter({ hasText: 'API' });
	await expect(row.getByTestId('row-status')).toHaveText('Up');
	await expect(page.getByTestId('count-up')).toHaveText('1');
	await expect(page.getByTestId('live-status')).toHaveText('connected');

	// New checks arrive over server-sent events.
	const before = await row.locator('[data-beat]').count();
	await expect.poll(() => row.locator('[data-beat]').count(), { timeout: 15_000 }).toBeGreaterThan(before);
	expect(problems).toEqual([]);
});

test('an outage shows live on the admin and public pages, and recovers', async ({ page, browser }) => {
	const problems = watchConsole(page);
	await signIn(page);
	await page.getByTestId('monitor-row').filter({ hasText: 'API' }).click();
	await expect(page.getByTestId('detail-status')).toHaveText('Up');
	await mark(page);

	const visitor = await browser.newPage();
	const visitorProblems = watchConsole(visitor);
	await visitor.goto('/');
	await expect(visitor.getByTestId('overall')).toHaveText('All systems operational');
	await visitor.evaluate(() => ((window as any).__marker = true));

	await setTarget(page, 503);
	await expect(page.getByTestId('detail-status')).toHaveText('Down', { timeout: 15_000 });
	await expectSameWindow(page);
	await expect(page.getByTestId('events')).toContainText('HTTP 503, expected 200-399');

	// The public page turns red and shows the incident the outage opened, without a reload.
	await expect(visitor.getByTestId('overall')).toHaveText('Major outage', { timeout: 15_000 });
	await expect(visitor.getByTestId('incident').first()).toContainText('API is down');
	await expect(visitor.getByTestId('incident').first()).toContainText('HTTP 503');
	expect(await visitor.evaluate(() => (window as any).__marker)).toBe(true);

	await setTarget(page, 200);
	await expect(page.getByTestId('detail-status')).toHaveText('Up', { timeout: 15_000 });
	await expect(visitor.getByTestId('overall')).toHaveText('All systems operational', { timeout: 15_000 });
	// The incident was resolved and moved to past incidents.
	await expect(visitor.getByText('Past incidents')).toBeVisible();
	await expect(visitor.getByTestId('incident').first()).toContainText('is available again');

	expect(problems).toEqual([]);
	expect(visitorProblems).toEqual([]);
	await visitor.close();
});

test('notification channels can be tested, and outages notify them', async ({ page }) => {
	await signIn(page);
	await page.getByRole('link', { name: 'Notifications' }).click();
	await page.getByLabel('Name', { exact: true }).fill('Ops webhook');
	await page.getByLabel('Webhook URL').fill('http://localhost:4176/api/test/webhook');
	await page.getByRole('button', { name: 'Add channel' }).click();
	const channel = page.getByTestId('channel').filter({ hasText: 'Ops webhook' });
	await expect(channel).toBeVisible();

	await channel.getByRole('button', { name: 'Test' }).click();
	await expect(page.getByText('Sent a test notification to Ops webhook.')).toBeVisible();
	const received = async () => (await (await page.request.get('/api/test/webhook')).json()) as { title: string; event: string }[];
	expect((await received()).at(-1)?.title).toContain('Test monitor is down');

	// A real outage is delivered by a background job.
	await setTarget(page, 500);
	await expect.poll(async () => (await received()).some((m) => m.title === '🔴 API is down'), { timeout: 20_000 }).toBe(true);
	await setTarget(page, 200);
	await expect.poll(async () => (await received()).some((m) => m.title === '🟢 API is back up'), { timeout: 20_000 }).toBe(true);
	await page.reload();
	await expect(page.getByText('API is up').first()).toBeVisible();
});

test('email notifications are rendered from a Svelte template', async ({ page }) => {
	const smtp = await smtpSink();
	try {
		await signIn(page);
		await page.getByRole('link', { name: 'Notifications' }).click();
		await page.getByLabel('Type').selectOption('email');
		await page.getByLabel('Name', { exact: true }).fill('Ops email');
		await page.getByLabel('SMTP host').fill('127.0.0.1');
		await page.getByLabel('Port').fill(String(smtp.port));
		await page.getByLabel('From').fill('Status <status@example.test>');
		await page.getByLabel('To (comma-separated)').fill('ops@example.test');
		await page.getByRole('button', { name: 'Add channel' }).click();
		const channel = page.getByTestId('channel').filter({ hasText: 'Ops email' });
		await channel.getByRole('button', { name: 'Test' }).click();
		await expect(page.getByText('Sent a test notification to Ops email.')).toBeVisible();

		expect(smtp.messages).toHaveLength(1);
		const raw = smtp.messages[0]!;
		expect(raw).toContain('To: <ops@example.test>');
		expect(raw).toMatch(/Subject: =\?utf-8\?q\?.*Test_monitor_is_down/i);
		expect(raw).toContain('multipart/alternative');
		// The HTML part comes from src/emails/MonitorAlert.svelte, with its styles.
		const decoded = raw.replace(/=\r\n/g, '').replace(/=([0-9A-F]{2})/g, (_, h) => String.fromCharCode(parseInt(h, 16)));
		expect(decoded).toContain('Open the status page');
		expect(decoded).toContain('<style');
		expect(decoded).toContain('this is a test notification');
		// The plain-text part is made from the HTML.
		expect(decoded).toMatch(/text\/plain[\s\S]*Test monitor is down/);

		await channel.getByRole('button', { name: 'Delete Ops email' }).click();
		await expect(channel).toHaveCount(0);
	} finally {
		await smtp.close();
	}
});

test('incidents are posted with Markdown and resolved', async ({ page, browser }) => {
	await signIn(page);
	await page.getByRole('link', { name: 'Incidents' }).click();
	await page.getByLabel('Title', { exact: true }).fill('Database upgrade');
	await page.getByLabel('Impact').selectOption('maintenance');
	await page.getByLabel('Message').fill('We are upgrading the **database**. <script>alert(1)</script>');
	await page.getByRole('button', { name: 'Open incident' }).click();
	await expect(page).toHaveURL(/\/admin\/incidents\/\d+$/);

	const visitor = await browser.newPage();
	await visitor.goto('/');
	await expect(visitor.getByTestId('overall')).toHaveText('Scheduled maintenance in progress');
	const incident = visitor.getByTestId('incident').filter({ hasText: 'Database upgrade' });
	await expect(incident.locator('strong')).toHaveText('database');
	expect(await incident.innerHTML()).not.toContain('<script>');

	await page.getByLabel('Status', { exact: true }).selectOption('resolved');
	await page.getByLabel('Update').fill('The upgrade is done.');
	await page.getByRole('button', { name: 'Post update' }).click();
	await expect(page.getByText('The upgrade is done.')).toBeVisible();
	await expect(page.getByLabel('Update')).toHaveValue('');

	await visitor.reload();
	await expect(visitor.getByTestId('overall')).toHaveText('All systems operational');
	await visitor.close();
});

test('monitors can be paused, and badges and settings work', async ({ page }) => {
	await signIn(page);
	await page.getByTestId('monitor-row').filter({ hasText: 'API' }).click();
	await page.getByRole('button', { name: 'Pause' }).click();
	await expect(page.getByTestId('detail-status')).toHaveText('Paused');
	await page.goto('/');
	await expect(page.getByTestId('public-monitor').filter({ hasText: 'API' }).getByTestId('public-status')).toHaveText('Paused');
	await page.goBack();
	await page.getByRole('button', { name: 'Resume' }).click();
	await expect(page.getByTestId('detail-status')).not.toHaveText('Paused');

	const id = page.url().split('/').at(-1);
	const badge = await page.request.get(`/api/badge/${id}`);
	expect(badge.headers()['content-type']).toBe('image/svg+xml');
	expect(await badge.text()).toContain('API');
	expect((await page.request.get('/api/badge/9999')).status()).toBe(404);

	await page.getByRole('link', { name: 'Settings' }).click();
	await page.getByLabel('Title', { exact: true }).fill('Acme Status');
	await page.getByLabel('Description').fill('Live status of Acme services.');
	await page.getByRole('button', { name: 'Save' }).click();
	await expect(page.getByText('Saved.')).toBeVisible();
	await page.goto('/');
	await expect(page).toHaveTitle('Acme Status');
	await expect(page.getByText('Live status of Acme services.')).toBeVisible();
});

test('the public page is server-rendered and the admin stream needs a session', async ({ request }) => {
	const html = await (await request.get('/')).text();
	expect(html).toContain('data-testid="public-monitor"');
	expect(html).toContain('API');
	expect((await request.get('/api/events')).status()).toBe(401);
	expect((await request.get('/api/public/events', { timeout: 2000 }).catch(() => null))?.status() ?? 200).toBe(200);
});

test('the live dashboard refuses visitors who are not signed in', async ({ baseURL }) => {
	const { createHash } = await import('node:crypto');
	const id = 'c' + createHash('sha256').update('src/lib/LiveMonitors.svelte').digest('hex').slice(0, 10);
	const ws = new WebSocket(baseURL!.replace('http', 'ws') + '/_tern/live');
	const reply = await new Promise<{ op: string; error?: string; set?: unknown }>((resolve, reject) => {
		ws.onopen = () => ws.send(JSON.stringify({ op: 'mount', ref: 1, id, props: {} }));
		ws.onmessage = (e) => resolve(JSON.parse(String(e.data)));
		ws.onerror = () => reject(new Error('socket error'));
	});
	ws.close();
	expect(reply).toEqual({ op: 'error', ref: 1, error: 'Sign in to see the monitors.' });
});
