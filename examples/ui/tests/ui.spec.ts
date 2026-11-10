import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';

// Audits run without animations, so nothing is measured halfway through one.
test.use({ reducedMotion: 'reduce' });

/** Fails on any axe violation, listing them. */
async function audit(page: Page) {
	const { violations } = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice']).analyze();
	expect(violations.map((v) => `${v.id}: ${v.help} (${v.nodes.map((n) => n.target.join(' ')).join(', ')})`)).toEqual([]);
}

function watchConsole(page: Page) {
	const problems: string[] = [];
	page.on('console', (m) => (m.type() === 'error' || m.type() === 'warning') && problems.push(m.text()));
	page.on('pageerror', (e) => problems.push(e.message));
	return problems;
}

for (const theme of ['light', 'dark']) {
	test(`the gallery passes axe in ${theme} mode`, async ({ page }) => {
		const problems = watchConsole(page);
		await page.emulateMedia({ colorScheme: theme as 'light' | 'dark' });
		await page.goto('/');
		if (theme === 'dark') await page.getByTestId('theme').click();
		await expect(page.locator('html')).toHaveAttribute('data-theme', theme);
		await audit(page);
		// Open widgets are audited too.
		await page.getByRole('button', { name: 'Actions' }).click();
		await page.getByRole('button', { name: 'What is Tern UI?' }).click();
		await audit(page);
		// The avatar's missing image is on purpose: it falls back to initials.
		expect(problems.filter((p) => !p.includes('404'))).toEqual([]);
	});
}

test('the dialog traps focus, closes on Escape and returns focus', async ({ page }) => {
	await page.goto('/');
	const opener = page.getByTestId('open-dialog');
	await opener.click();
	const dialog = page.getByRole('dialog', { name: 'Delete this project?' });
	await expect(dialog).toBeVisible();
	await expect(dialog).toHaveAccessibleDescription(/can't be undone/);
	await audit(page);
	// Focus stays inside while tabbing.
	for (let i = 0; i < 6; i++) {
		await page.keyboard.press('Tab');
		expect(await dialog.evaluate((d) => d.contains(document.activeElement))).toBe(true);
	}
	await page.keyboard.press('Escape');
	await expect(dialog).toBeHidden();
	await expect(opener).toBeFocused();
	// Confirming needs the name.
	await opener.click();
	const del = dialog.getByRole('button', { name: 'Delete' });
	await expect(del).toBeDisabled();
	await page.getByTestId('confirm-name').fill('acme-site');
	await del.click();
	await expect(page.getByTestId('deleted')).toHaveText('Deleted acme-site.');
	await expect(page.getByRole('region', { name: 'Notifications' })).toContainText('Project deleted');
});

test('tabs follow the ARIA pattern', async ({ page }) => {
	await page.goto('/');
	const overview = page.getByRole('tab', { name: 'Overview' });
	await overview.focus();
	await page.keyboard.press('ArrowRight');
	await expect(page.getByRole('tab', { name: 'Activity' })).toBeFocused();
	await expect(page.getByRole('tab', { name: 'Activity' })).toHaveAttribute('aria-selected', 'true');
	await expect(page.getByRole('tabpanel', { name: 'Activity' })).toContainText('activity panel');
	// Disabled tabs are skipped; End and Home jump.
	await page.keyboard.press('End');
	await expect(page.getByRole('tab', { name: 'Settings' })).toBeFocused();
	await page.keyboard.press('ArrowRight');
	await expect(overview).toBeFocused();
	await page.keyboard.press('ArrowLeft');
	await expect(page.getByRole('tab', { name: 'Settings' })).toBeFocused();
	await page.keyboard.press('Home');
	await expect(overview).toBeFocused();
	// Only the selected tab is in the tab order.
	await expect(page.getByRole('tab', { name: 'Activity' })).toHaveAttribute('tabindex', '-1');
});

test('the menu works with the keyboard', async ({ page }) => {
	await page.goto('/');
	const button = page.getByRole('button', { name: 'Actions' });
	await button.focus();
	await page.keyboard.press('ArrowDown');
	const menu = page.getByRole('menu', { name: 'Actions' });
	await expect(menu).toBeVisible();
	await expect(button).toHaveAttribute('aria-expanded', 'true');
	await expect(page.getByRole('menuitem', { name: 'Rename' })).toBeFocused();
	await page.keyboard.press('ArrowDown');
	await expect(page.getByRole('menuitem', { name: 'Duplicate' })).toBeFocused();
	// Disabled items are skipped.
	await page.keyboard.press('ArrowDown');
	await expect(page.getByRole('menuitem', { name: 'Delete' })).toBeFocused();
	// Typing jumps to an item.
	await page.keyboard.press('r');
	await expect(page.getByRole('menuitem', { name: 'Rename' })).toBeFocused();
	await page.keyboard.press('Enter');
	await expect(menu).toBeHidden();
	await expect(button).toBeFocused();
	await expect(page.getByTestId('last-action')).toHaveText('rename');
	// Escape closes; a click outside does too.
	await page.keyboard.press('ArrowUp');
	await expect(page.getByRole('menuitem', { name: 'Delete' })).toBeFocused();
	await page.keyboard.press('Escape');
	await expect(menu).toBeHidden();
	await button.click();
	await page.mouse.click(5, 5);
	await expect(menu).toBeHidden();
});

test('form controls, tooltip, accordion and toasts', async ({ page }) => {
	const problems = watchConsole(page);
	await page.goto('/');
	const sw = page.getByRole('switch', { name: 'Email me when a monitor goes down' });
	await expect(sw).toHaveAttribute('aria-checked', 'true');
	await sw.focus();
	await page.keyboard.press('Space');
	await expect(sw).toHaveAttribute('aria-checked', 'false');
	await page.getByRole('checkbox', { name: 'Share anonymous usage data' }).check();
	await page.getByRole('radio', { name: 'Hobby' }).check();
	await expect(page.getByRole('radio', { name: 'Enterprise' })).toBeDisabled();
	await expect(page.getByTestId('choices')).toHaveText('Plan: hobby · notify: off · share: yes');
	// Fields wire up labels, descriptions and errors.
	await expect(page.getByRole('textbox', { name: 'Project name' })).toHaveAccessibleDescription('Shown on your status page.');
	await expect(page.getByRole('textbox', { name: 'Email' })).toHaveAttribute('aria-invalid', 'true');
	await expect(page.getByRole('textbox', { name: 'Email' })).toHaveAccessibleDescription(/doesn't look like an email/);
	await page.getByRole('combobox', { name: 'Region' }).selectOption('eu');

	// The tooltip describes its trigger on focus, and Escape hides it.
	const copy = page.getByRole('button', { name: 'Copy link' });
	await copy.focus();
	await expect(page.getByRole('tooltip')).toBeVisible();
	await expect(copy).toHaveAccessibleDescription('Copies the link to this project');
	await page.keyboard.press('Escape');
	await expect(page.getByRole('tooltip')).toBeHidden();

	const q = page.getByRole('button', { name: 'Can I change the colors?' });
	await q.click();
	await expect(q).toHaveAttribute('aria-expanded', 'true');
	await expect(page.getByRole('region', { name: 'Can I change the colors?' })).toContainText('--tern-*');
	await q.click();
	await expect(page.getByRole('region', { name: 'Can I change the colors?' })).toBeHidden();

	// A loading button stays focusable and says it's busy.
	const save = page.getByTestId('save');
	await save.click();
	await expect(save).toHaveAttribute('aria-busy', 'true');
	await expect(page.getByRole('region', { name: 'Notifications' })).toContainText('Changes saved');
	await page.getByRole('button', { name: 'Dismiss' }).first().click();
	await expect(page.getByRole('region', { name: 'Notifications' })).not.toContainText('Changes saved');

	await expect(page.getByRole('progressbar', { name: 'Uploading backup' })).toHaveAttribute('aria-valuenow', '40');
	await page.getByRole('button', { name: 'Advance' }).click();
	await expect(page.getByRole('progressbar', { name: 'Uploading backup' })).toHaveAttribute('aria-valuenow', '60');
	// The avatar falls back to initials with the name as its label.
	await expect(page.getByRole('img', { name: 'Alan Turing' })).toHaveText('AT');
	expect(problems.filter((p) => !p.includes('missing.png') && !p.includes('404'))).toEqual([]);
});

test('the form shows Go validation errors, with and without JavaScript', async ({ browser }) => {
	for (const javaScriptEnabled of [true, false]) {
		const page = await (await browser.newContext({ javaScriptEnabled })).newPage();
		await page.goto('/form');
		if (javaScriptEnabled) await audit(page);
		await page.getByRole('textbox', { name: 'Email' }).fill('nope');
		await page.getByRole('button', { name: 'Create account' }).click();
		await expect(page.getByText('4 fields need your attention.')).toBeVisible();
		const email = page.getByRole('textbox', { name: 'Email' });
		await expect(email).toHaveAttribute('aria-invalid', 'true');
		await expect(email).toHaveValue('nope');
		await expect(email).toHaveAccessibleDescription(/only email you about your account.*like ada@example.com/);
		if (javaScriptEnabled) await audit(page);

		await page.getByRole('textbox', { name: 'Name' }).fill('Ada');
		await email.fill('ada@example.com');
		await page.getByRole('combobox', { name: 'Role' }).selectOption('dev');
		await page.getByRole('checkbox', { name: 'I accept the terms' }).check();
		await page.getByRole('button', { name: 'Create account' }).click();
		await expect(page.getByText("You're in")).toBeVisible();
		await expect(page.getByText('Welcome, Ada.')).toBeVisible();
		await page.close();
	}
});
