import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';

test.use({ reducedMotion: 'reduce' });

async function audit(page: Page) {
	const { violations } = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice']).analyze();
	expect(violations.map((v) => `${v.id}: ${v.help} (${v.nodes.map((n) => n.target.join(' ')).join(', ')})`)).toEqual([]);
}

for (const theme of ['light', 'dark'] as const) {
	test(`overlays pass axe in ${theme} mode, open and closed`, async ({ page }) => {
		const problems: string[] = [];
		page.on('console', (m) => (m.type() === 'error' || m.type() === 'warning') && problems.push(m.text()));
		page.on('pageerror', (e) => problems.push(e.message));
		await page.emulateMedia({ colorScheme: theme });
		await page.goto('/overlays');
		if (theme === 'dark') await page.getByTestId('theme').click();
		await audit(page);
		await page.getByRole('menuitem', { name: 'File' }).click();
		await page.getByRole('menuitem', { name: 'Open recent' }).press('ArrowRight');
		await expect(page.getByRole('menuitem', { name: 'budget.csv' })).toBeFocused();
		await audit(page);
		await page.keyboard.press('Escape');
		await page.keyboard.press('Escape');
		await page.getByTestId('popover-trigger').click();
		await audit(page);
		await page.keyboard.press('Escape');
		await page.getByTestId('open-sheet').click();
		await audit(page);
		expect(problems).toEqual([]);
	});
}

test('the menubar follows the ARIA pattern', async ({ page }) => {
	await page.goto('/overlays');
	const file = page.getByRole('menuitem', { name: 'File' });
	await file.focus();
	// Arrow keys move along the bar; only one item is in the tab order.
	await page.keyboard.press('ArrowRight');
	await expect(page.getByRole('menuitem', { name: 'Edit' })).toBeFocused();
	await expect(file).toHaveAttribute('tabindex', '-1');
	await page.keyboard.press('ArrowLeft');
	await page.keyboard.press('Enter');
	await expect(page.getByRole('menuitem', { name: /New file/ })).toBeFocused();
	// Disabled items are skipped.
	await page.keyboard.press('End');
	await expect(page.getByRole('menuitem', { name: 'Close window' })).toBeFocused();
	await page.keyboard.press('ArrowUp');
	await expect(page.getByRole('menuitem', { name: /Open recent/ })).toBeFocused();
	// Submenus open with ArrowRight and close with ArrowLeft.
	await page.keyboard.press('ArrowRight');
	await expect(page.getByRole('menuitem', { name: 'budget.csv' })).toBeFocused();
	await page.keyboard.press('ArrowLeft');
	await expect(page.getByRole('menuitem', { name: /Open recent/ })).toBeFocused();
	await expect(page.getByRole('menu', { name: 'Open recent' })).toHaveCount(0);
	// From a plain item, ArrowRight opens the next menu.
	await page.keyboard.press('Home');
	await page.keyboard.press('ArrowRight');
	await expect(page.getByRole('menuitem', { name: /Undo/ })).toBeFocused();
	await page.keyboard.press('ArrowRight');
	const wrap = page.getByRole('menuitemcheckbox', { name: 'Word wrap' });
	await expect(wrap).toBeFocused();
	await expect(wrap).toHaveAttribute('aria-checked', 'true');
	await expect(page.getByRole('group', { name: 'Zoom' }).getByRole('menuitemradio')).toHaveCount(3);
	await page.keyboard.press('Enter');
	await expect(page.getByRole('menuitem', { name: 'View' })).toBeFocused();
	await expect(page.getByTestId('last')).toContainText('wrap off');
	// Typing jumps to an item.
	await page.keyboard.press('ArrowLeft');
	await page.keyboard.press('ArrowLeft');
	await page.keyboard.press('Enter');
	await page.keyboard.type('c');
	await expect(page.getByRole('menuitem', { name: 'Close window' })).toBeFocused();
	await page.keyboard.press('Enter');
	await expect(page.getByTestId('last')).toContainText('Close window');
});

test('menus close on outside clicks and Tab', async ({ page }) => {
	await page.goto('/overlays');
	await page.getByRole('button', { name: 'View options' }).click();
	await expect(page.getByRole('menu')).toBeVisible();
	await page.getByRole('heading', { name: 'Overlays' }).click();
	await expect(page.getByRole('menu')).toHaveCount(0);
	await page.getByRole('button', { name: 'View options' }).press('Enter');
	await page.keyboard.press('Tab');
	await expect(page.getByRole('menu')).toHaveCount(0);
});

test('the context menu opens with the keyboard and returns focus', async ({ page }) => {
	await page.goto('/overlays');
	const area = page.getByTestId('context-area');
	await area.focus();
	await page.keyboard.press('Shift+F10');
	const menu = page.getByRole('menu', { name: 'Card actions' });
	await expect(menu).toBeVisible();
	await expect(page.getByRole('menuitem', { name: /Copy link/ })).toBeFocused();
	await page.keyboard.press('ArrowDown');
	await page.keyboard.press('Enter');
	await expect(area).toBeFocused();
	await expect(page.getByTestId('last')).toContainText('Duplicate');
	await area.click({ button: 'right' });
	await expect(menu).toBeVisible();
	await page.keyboard.press('Escape');
	await expect(menu).toHaveCount(0);
});

test('the popover moves focus in and back out', async ({ page }) => {
	await page.goto('/overlays');
	const trigger = page.getByTestId('popover-trigger');
	await trigger.click();
	await expect(trigger).toHaveAttribute('aria-expanded', 'true');
	const popover = page.getByRole('dialog', { name: 'Dimensions' });
	await expect(popover).toBeVisible();
	await expect(page.getByTestId('popover-width')).toBeFocused();
	await page.getByTestId('popover-width').fill('480');
	await expect(page.getByTestId('width')).toHaveText('Width: 480px');
	await page.keyboard.press('Escape');
	await expect(popover).toHaveCount(0);
	await expect(trigger).toBeFocused();
	// The trigger toggles it.
	await trigger.click();
	await trigger.click();
	await expect(popover).toHaveCount(0);
});

test('the hover card opens on focus and closes on Escape', async ({ page }) => {
	await page.goto('/overlays');
	const link = page.getByRole('link', { name: '@ternkit' });
	await link.focus();
	await expect(page.getByText('Svelte on the client, Go on the server.')).toBeVisible();
	await page.keyboard.press('Escape');
	await expect(page.getByText('Svelte on the client, Go on the server.')).toBeHidden();
});

test('the alert dialog starts on Cancel and waits for the action', async ({ page }) => {
	await page.goto('/overlays');
	const opener = page.getByTestId('open-alert');
	await opener.click();
	const alert = page.getByRole('alertdialog', { name: 'Remove Ada from the team?' });
	await expect(alert).toBeVisible();
	await expect(alert.getByRole('button', { name: 'Cancel' })).toBeFocused();
	await page.keyboard.press('Escape');
	await expect(alert).toBeHidden();
	await expect(opener).toBeFocused();
	await opener.click();
	await alert.getByRole('button', { name: 'Remove' }).click();
	await expect(alert).toBeHidden();
	await expect(page.getByTestId('removed')).toBeVisible();
	await expect(opener).toBeFocused();
});

test('sheets and drawers are modal and close by dragging', async ({ page }) => {
	await page.goto('/overlays');
	await page.getByTestId('open-sheet').click();
	const sheet = page.getByRole('dialog', { name: 'Filters' });
	await expect(sheet).toBeVisible();
	const box = await sheet.boundingBox();
	const width = page.viewportSize()!.width;
	expect(Math.round(box!.x + box!.width)).toBe(width); // against the right edge
	for (let i = 0; i < 5; i++) {
		await page.keyboard.press('Tab');
		expect(await sheet.evaluate((d) => d.contains(document.activeElement))).toBe(true);
	}
	await page.keyboard.press('Escape');
	await page.getByTestId('open-drawer').click();
	const drawer = page.getByRole('dialog', { name: 'Share' });
	await expect(drawer).toBeVisible();
	const handle = drawer.locator('.handle');
	const h = (await handle.boundingBox())!;
	await page.mouse.move(h.x + h.width / 2, h.y + h.height / 2);
	await page.mouse.down();
	await page.mouse.move(h.x + h.width / 2, h.y + 300, { steps: 6 });
	await page.mouse.up();
	await expect(drawer).toBeHidden();
	await expect(page.getByTestId('open-drawer')).toBeFocused();
});
