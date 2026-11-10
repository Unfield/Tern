import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';

test.use({ reducedMotion: 'reduce' });

async function audit(page: Page) {
	const { violations } = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice']).analyze();
	expect(violations.map((v) => `${v.id}: ${v.help} (${v.nodes.map((n) => n.target.join(' ')).join(', ')})`)).toEqual([]);
}

for (const theme of ['light', 'dark'] as const) {
	test(`inputs pass axe in ${theme} mode, open and closed`, async ({ page }) => {
		const problems: string[] = [];
		page.on('console', (m) => (m.type() === 'error' || m.type() === 'warning') && problems.push(m.text()));
		page.on('pageerror', (e) => problems.push(e.message));
		await page.emulateMedia({ colorScheme: theme });
		await page.goto('/inputs');
		if (theme === 'dark') await page.getByTestId('theme').click();
		await audit(page);
		await page.getByRole('combobox', { name: 'Language' }).click();
		await audit(page);
		await page.keyboard.press('Escape');
		await page.getByRole('combobox', { name: /Region/ }).click();
		await audit(page);
		await page.keyboard.press('Escape');
		await page.getByRole('button', { name: /Due date/ }).click();
		await audit(page);
		await page.keyboard.press('Escape');
		await page.getByTestId('open-palette').click();
		await audit(page);
		expect(problems).toEqual([]);
	});
}

test('the combobox filters, skips disabled options and reverts on blur', async ({ page }) => {
	await page.goto('/inputs');
	const box = page.getByRole('combobox', { name: 'Language' });
	await box.fill('t');
	await expect(box).toHaveAttribute('aria-expanded', 'true');
	const list = page.getByRole('listbox', { name: 'Language' });
	await expect(list.getByRole('option')).toHaveText(['TypeScript']);
	await box.fill('');
	await box.press('Escape');
	await box.press('ArrowDown'); // opens on the first option
	await expect(list.getByRole('group', { name: 'Compiled' })).toBeVisible();
	// Go is first; ArrowDown goes to Rust, then past disabled Zig to TypeScript.
	await box.press('ArrowDown');
	await box.press('ArrowDown');
	const active = await box.getAttribute('aria-activedescendant');
	await expect(page.locator(`#${active}`)).toHaveText('TypeScript');
	await box.press('Enter');
	await expect(box).toHaveValue('TypeScript');
	await expect(page.getByTestId('chosen')).toContainText('Language: ts');
	// Typing something else and leaving keeps the chosen option.
	await box.fill('Pyt');
	await page.getByRole('heading', { name: 'Inputs' }).click();
	await expect(box).toHaveValue('TypeScript');
	// Escape on a closed box clears it.
	await box.focus();
	await box.press('Escape');
	await expect(page.getByTestId('chosen')).toContainText('Language: none');
});

test('the combobox searches a Go endpoint', async ({ page }) => {
	await page.goto('/inputs');
	const box = page.getByRole('combobox', { name: 'Owner' });
	await box.fill('perl');
	await expect(page.getByRole('option', { name: /Radia Perlman/ })).toBeVisible();
	await box.press('Enter');
	await expect(page.getByTestId('chosen')).toContainText('owner: radia');
	await box.fill('zzz');
	await expect(page.getByText('Nobody found')).toBeVisible();
});

test('the select menu works like a native select', async ({ page }) => {
	await page.goto('/inputs');
	const select = page.getByRole('combobox', { name: /Region/ });
	await expect(select).toHaveAccessibleName('Region Europe');
	await select.focus();
	// Typing picks while closed, like a native select.
	await page.keyboard.type('u');
	await expect(page.getByTestId('chosen')).toContainText('region: us');
	await page.keyboard.press('Enter');
	await expect(select).toHaveAttribute('aria-expanded', 'true');
	await page.keyboard.press('ArrowDown');
	await page.keyboard.press('Enter');
	await expect(select).toHaveAttribute('aria-expanded', 'false');
	await expect(select).toBeFocused();
	await expect(page.getByTestId('chosen')).toContainText('region: ap');
	await select.click();
	await page.getByRole('option', { name: /Europe/ }).click();
	await expect(page.getByTestId('chosen')).toContainText('region: eu');
});

test('the calendar follows the date grid pattern', async ({ page }) => {
	await page.goto('/inputs');
	const grid = page.getByRole('grid', { name: 'November 2026' });
	const day = grid.getByRole('button', { name: 'Tuesday, 10 November 2026' });
	await expect(day).toHaveAttribute('tabindex', '0');
	await day.focus();
	await page.keyboard.press('ArrowRight');
	await expect(grid.getByRole('button', { name: 'Wednesday, 11 November 2026' })).toBeFocused();
	await page.keyboard.press('ArrowDown');
	await expect(grid.getByRole('button', { name: 'Wednesday, 18 November 2026' })).toBeFocused();
	await page.keyboard.press('Home'); // weeks start on Monday here
	await expect(grid.getByRole('button', { name: 'Monday, 16 November 2026' })).toBeFocused();
	await page.keyboard.press('PageDown');
	await expect(page.getByRole('grid', { name: 'December 2026' }).getByRole('button', { name: 'Wednesday, 16 December 2026' })).toBeFocused();
	await page.keyboard.press('Enter');
	await expect(page.getByTestId('dates')).toContainText('day: 2026-12-16');
	// Weekends are disabled here.
	await page.keyboard.press('End');
	await page.keyboard.press('Enter');
	await expect(page.getByTestId('dates')).toContainText('day: 2026-12-16');
});

test('the date picker opens a calendar and posts ISO dates', async ({ page }) => {
	await page.goto('/inputs');
	const trigger = page.getByRole('button', { name: /Due date/ });
	await trigger.click();
	const dialog = page.getByRole('dialog', { name: 'Choose a date' });
	await expect(dialog).toBeVisible();
	await expect(dialog.getByRole('grid')).toBeVisible();
	await dialog.getByRole('button', { name: 'Next month' }).click();
	await dialog.getByRole('gridcell').getByRole('button', { name: /^\w+,? .*15/ }).first().click();
	await expect(dialog).toBeHidden();
	await expect(trigger).toBeFocused();
	await expect(page.getByTestId('dates')).toContainText(/Due: \d{4}-\d{2}-15/);
	await expect(trigger).toHaveAccessibleName(/Due date .*15/);
	// A range takes two clicks.
	const trip = page.getByRole('button', { name: /Trip/ });
	await trip.click();
	const cal = page.getByRole('dialog', { name: 'Choose a date' });
	const days = cal.locator('button[data-date]:not(.outside)');
	await days.nth(3).click();
	await expect(cal).toBeVisible();
	await days.nth(8).click();
	await expect(cal).toBeHidden();
	await expect(page.getByTestId('dates')).toContainText(/trip: \d{4}-\d{2}-04 to \d{4}-\d{2}-09/);
});

test('sliders move with keys and pointer, and ranges never cross', async ({ page }) => {
	await page.goto('/inputs');
	const volume = page.getByRole('slider', { name: 'Volume' });
	await expect(volume).toHaveAttribute('aria-valuetext', '40%');
	await volume.focus();
	await page.keyboard.press('ArrowRight');
	await page.keyboard.press('PageUp');
	await expect(page.getByTestId('sliders')).toContainText('Volume 51');
	await page.keyboard.press('End');
	await expect(volume).toHaveAttribute('aria-valuenow', '100');
	const low = page.getByRole('slider', { name: 'Price minimum' });
	const high = page.getByRole('slider', { name: 'Price maximum' });
	await low.focus();
	await page.keyboard.press('End'); // stops at the high thumb
	await expect(low).toHaveAttribute('aria-valuenow', '80');
	await expect(high).toHaveAttribute('aria-valuemin', '80');
	// Clicking the track moves the nearest thumb.
	const track = page.locator('.tern-slider .track').nth(1);
	const b = (await track.boundingBox())!;
	await page.mouse.click(b.x + b.width * 0.1, b.y + b.height / 2);
	await expect(page.getByTestId('sliders')).toContainText('price 20 to 80');
});

test('toggle groups act as radios or toggles', async ({ page }) => {
	await page.goto('/inputs');
	const center = page.getByRole('radio', { name: 'Center' });
	await page.getByRole('radio', { name: 'Left' }).focus();
	await page.keyboard.press('ArrowRight');
	await expect(center).toBeFocused();
	await expect(center).toHaveAttribute('aria-checked', 'true');
	await expect(page.getByRole('radio', { name: 'Left' })).toHaveAttribute('tabindex', '-1');
	await page.getByRole('button', { name: 'Italic' }).click();
	await expect(page.getByRole('button', { name: 'Italic' })).toHaveAttribute('aria-pressed', 'true');
	await page.getByRole('button', { name: 'Bold', exact: true }).last().click();
	await expect(page.getByTestId('toggles')).toContainText('styles italic');
	await page.getByRole('button', { name: 'Bold', exact: true }).first().click();
	await expect(page.getByTestId('toggles')).toContainText('Bold on');
});

test('the password field shows and hides, and the code field takes a paste', async ({ page }) => {
	await page.goto('/inputs');
	const password = page.getByLabel('Password', { exact: true });
	await expect(password).toHaveAttribute('type', 'password');
	await expect(password).toHaveAttribute('autocomplete', 'new-password');
	await password.fill('correct horse');
	const toggle = page.getByRole('button', { name: 'Show password' });
	await expect(toggle).toHaveAttribute('aria-pressed', 'false');
	await toggle.click();
	await expect(password).toHaveAttribute('type', 'text');
	await expect(toggle).toHaveAttribute('aria-pressed', 'true');
	await toggle.click();
	await expect(password).toHaveAttribute('type', 'password');
	await expect(page.getByTestId('text')).toContainText('password length 13');

	const code = page.getByLabel('Verification code');
	await expect(code).toHaveAttribute('autocomplete', 'one-time-code');
	await code.fill('12a3456789');
	await expect(code).toHaveValue('123456');
	await expect(page.getByTestId('text')).toContainText('Code: 123456');
	await expect(page.getByRole('region', { name: 'Notifications' })).toContainText('Code 123456 entered');
});

test('the command palette filters and runs, inline and with ⌘K', async ({ page }) => {
	await page.goto('/inputs');
	const inline = page.getByRole('combobox', { name: 'Commands' });
	await inline.fill('pref');
	await expect(page.getByRole('listbox', { name: 'Commands' }).getByRole('option')).toHaveText([/Settings/]);
	await inline.press('Enter');
	await expect(page.getByTestId('ran')).toHaveText('Ran: Settings');

	await page.getByRole('heading', { name: 'Inputs' }).click();
	await page.keyboard.press('ControlOrMeta+k');
	const palette = page.getByRole('dialog', { name: 'Command palette' });
	await expect(palette).toBeVisible();
	await expect(palette.getByRole('combobox')).toBeFocused();
	await page.keyboard.press('ArrowDown');
	await page.keyboard.press('Enter');
	await expect(palette).toBeHidden();
	await expect(page.getByTestId('ran')).toHaveText('Ran: Open project');
	await page.keyboard.press('ControlOrMeta+k');
	await page.keyboard.press('Escape');
	await expect(palette).toBeHidden();
});
