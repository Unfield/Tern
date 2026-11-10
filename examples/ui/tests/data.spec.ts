import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';

test.use({ reducedMotion: 'reduce' });

async function audit(page: Page) {
	const { violations } = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa', 'best-practice']).analyze();
	expect(violations.map((v) => `${v.id}: ${v.help} (${v.nodes.map((n) => n.target.join(' ')).join(', ')})`)).toEqual([]);
}

for (const path of ['/data', '/layout']) {
	for (const theme of ['light', 'dark'] as const) {
		test(`${path} passes axe in ${theme} mode`, async ({ page }) => {
			const problems: string[] = [];
			page.on('console', (m) => (m.type() === 'error' || m.type() === 'warning') && problems.push(m.text()));
			page.on('pageerror', (e) => problems.push(e.message));
			await page.emulateMedia({ colorScheme: theme });
			await page.goto(path);
			if (theme === 'dark') await page.getByTestId('theme').click();
			await audit(page);
			if (path === '/data') {
				await page.getByRole('group', { name: /^Traffic/ }).focus();
				await page.getByRole('button', { name: 'Show data' }).first().click();
			} else {
				await page.getByRole('button', { name: 'Product' }).click();
			}
			await audit(page);
			expect(problems).toEqual([]);
		});
	}
}

test('charts read out values from the keyboard and have tables', async ({ page }) => {
	await page.goto('/data');
	const traffic = page.getByRole('group', { name: /^Traffic/ });
	await traffic.focus();
	const figure = page.getByRole('figure', { name: 'Traffic' });
	// Focus starts at the latest point; arrows move along the days.
	await expect(figure.locator('[aria-live]')).toContainText('Pageviews');
	const latest = await figure.locator('[aria-live]').textContent();
	await page.keyboard.press('ArrowLeft');
	await expect(figure.locator('[aria-live]')).not.toHaveText(latest!);
	await page.keyboard.press('Home');
	await expect(figure.locator('.tooltip')).toBeVisible();
	await page.keyboard.press('Escape');
	await expect(figure.locator('.tooltip')).toHaveCount(0);
	// The legend names both series, and the table has every day.
	await expect(figure.getByRole('list', { name: 'Legend' }).getByRole('listitem')).toHaveText(['Pageviews', 'Visitors']);
	await figure.getByRole('button', { name: 'Show data' }).click();
	await expect(figure.getByRole('table').getByRole('row')).toHaveCount(31);
	// Bars answer per category.
	const sources = page.getByRole('group', { name: /^Sources/ });
	await sources.focus();
	await expect(page.getByRole('figure', { name: 'Sources' }).locator('[aria-live]')).toContainText(/Search: Visits .*Sign-ups/);
	await page.keyboard.press('ArrowRight');
	await expect(page.getByRole('figure', { name: 'Sources' }).locator('[aria-live]')).toContainText('Direct');
	// The donut folds small parts into Other.
	await expect(page.getByRole('figure', { name: 'Browsers' }).getByRole('listitem')).toHaveCount(6);
	await expect(page.getByRole('figure', { name: 'Browsers' }).getByRole('listitem').last()).toContainText('Other');
});

test('the data table sorts, searches, selects and pages', async ({ page }) => {
	await page.goto('/data');
	const table = page.getByRole('table', { name: 'Top pages' });
	await expect(table.getByRole('row')).toHaveCount(6); // header + 5
	await expect(page.getByRole('status').filter({ hasText: 'of 13 rows' })).toHaveText('1–5 of 13 rows');
	const views = table.getByRole('columnheader', { name: /Views/ });
	await expect(views).toHaveAttribute('aria-sort', 'none');
	await views.getByRole('button').click();
	await expect(views).toHaveAttribute('aria-sort', 'ascending');
	await expect(table.getByRole('row').nth(1)).toContainText('/signup');
	await views.getByRole('button').click();
	await expect(views).toHaveAttribute('aria-sort', 'descending');
	await expect(table.getByRole('row').nth(1)).toContainText('/');
	await page.getByRole('searchbox', { name: 'Search Top pages' }).fill('docs');
	await expect(table.getByRole('row')).toHaveCount(5); // header + 4 docs pages
	await page.getByRole('searchbox', { name: 'Search Top pages' }).fill('');
	await page.getByRole('checkbox', { name: 'Select all rows on this page' }).check();
	await expect(page.getByTestId('selected')).not.toHaveText('Selected: none');
	await expect(page.getByRole('button', { name: 'Export selected' })).toBeEnabled();
	await page.getByRole('navigation', { name: 'Top pages pages' }).getByRole('button', { name: 'Next page' }).click();
	await expect(page.getByRole('status').filter({ hasText: 'of 13 rows' })).toContainText('6–10 of 13 rows');
	await expect(page.getByRole('checkbox', { name: 'Select all rows on this page' })).not.toBeChecked();
});

test('navigation, pagination, panes and the carousel', async ({ page }) => {
	await page.goto('/layout');
	// Breadcrumbs collapse the middle into a menu and mark the current page.
	const crumbs = page.getByRole('navigation', { name: 'Breadcrumb' });
	await expect(crumbs.locator('[aria-current="page"]')).toHaveText('Domains');
	await crumbs.getByRole('button', { name: /Show 2 more/ }).click();
	await expect(page.getByRole('menuitem', { name: 'Projects' })).toBeVisible();
	await page.keyboard.press('Escape');
	// The navigation menu is a disclosure with links.
	const product = page.getByRole('button', { name: 'Product' });
	await product.click();
	await expect(product).toHaveAttribute('aria-expanded', 'true');
	await page.keyboard.press('ArrowDown');
	await expect(page.getByRole('link', { name: /Live components/ })).toBeFocused();
	await page.keyboard.press('Escape');
	await expect(product).toBeFocused();
	await expect(product).toHaveAttribute('aria-expanded', 'false');
	// Pagination.
	const pager = page.getByRole('navigation', { name: 'Pagination' });
	await expect(pager.getByRole('button', { name: '4' })).toHaveAttribute('aria-current', 'page');
	await pager.getByRole('button', { name: '20' }).click();
	await expect(page.getByTestId('page')).toHaveText('Page 20');
	await expect(pager.getByRole('button', { name: 'Next page' })).toBeDisabled();
	// The splitter moves with the keyboard.
	const handle = page.getByRole('separator', { name: 'Resize the file list' });
	await handle.focus();
	await page.keyboard.press('ArrowRight');
	await expect(page.getByTestId('split')).toHaveText('The file list takes 40%.');
	await page.keyboard.press('End');
	await expect(handle).toHaveAttribute('aria-valuenow', '85');
	// The scroll area is reachable by keyboard.
	await expect(page.getByRole('region', { name: 'Files' })).toHaveAttribute('tabindex', '0');
	// The carousel hides slides out of view from the keyboard.
	const carousel = page.getByRole('region', { name: 'Why Tern' });
	await expect(carousel.getByRole('group', { name: '1 of 5' })).not.toHaveAttribute('inert');
	await expect(carousel.getByRole('group', { name: '3 of 5' })).toHaveAttribute('inert', '');
	await carousel.getByRole('button', { name: 'Next slide' }).click();
	await expect(carousel.getByRole('group', { name: '3 of 5' })).not.toHaveAttribute('inert');
	// The sidebar collapses to icons.
	await page.getByRole('button', { name: 'Collapse sidebar' }).click();
	await expect(page.getByRole('button', { name: 'Expand sidebar' })).toHaveAttribute('aria-pressed', 'true');
});

test('the sidebar becomes a sheet on phones', async ({ page }) => {
	await page.setViewportSize({ width: 390, height: 800 });
	await page.goto('/layout');
	const open = page.getByRole('button', { name: 'Open navigation' });
	await open.click();
	const sheet = page.getByRole('dialog', { name: 'Navigation' });
	await expect(sheet).toBeVisible();
	await expect(sheet.getByRole('link', { name: /Overview/ })).toHaveAttribute('aria-current', 'page');
	await page.keyboard.press('Escape');
	await expect(open).toBeFocused();
});
