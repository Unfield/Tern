import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';

test.use({ reducedMotion: 'reduce' });

let n = 0;
const newEmail = () => `user-${Date.now()}-${process.pid}-${n++}@example.com`;
const password = 'analytical engine';

async function audit(page: Page) {
	const { violations } = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa']).analyze();
	expect(violations.map((v) => `${v.id}: ${v.help} (${v.nodes.map((n) => n.target.join(' ')).join(', ')})`)).toEqual([]);
}

async function signUp(page: Page, email = newEmail()) {
	await page.goto('/signup');
	await page.getByRole('textbox', { name: 'Name' }).fill('Ada');
	await page.getByRole('textbox', { name: 'Email' }).fill(email);
	await page.locator('input[name=password]').fill(password);
	await page.getByRole('button', { name: 'Create account' }).click();
	await expect(page).toHaveURL(/\/account$/);
	return email;
}

async function signIn(page: Page, email: string, pw = password) {
	await page.goto('/login');
	await page.getByRole('textbox', { name: 'Email' }).fill(email);
	await page.locator('input[name=password]').fill(pw);
	await page.getByRole('button', { name: 'Sign in', exact: true }).click();
}

// The newest link the outbox has for email.
async function linkFor(page: Page, email: string, subject: string) {
	await page.goto('/outbox');
	const card = page.locator('.tern-card', { hasText: `To ${email}` }).filter({ hasText: subject }).first();
	return (await card.getByTestId('email-link').getAttribute('href'))!;
}

for (const path of ['/login', '/signup', '/forgot-password', '/reset-password?token=x']) {
	test(`${path} passes axe`, async ({ page }) => {
		await page.goto(path);
		await audit(page);
	});
}

test('sign up, sign out and back in', async ({ page }) => {
	const email = await signUp(page);
	await expect(page.getByText('Not confirmed')).toBeVisible();
	await audit(page);
	await page.getByRole('button', { name: 'Sign out' }).first().click();
	await expect(page).toHaveURL(/\/login$/);

	await signIn(page, email, 'not my password');
	await expect(page.getByText('Wrong email or password.')).toBeVisible();
	await signIn(page, email.toUpperCase());
	await expect(page).toHaveURL(/\/account$/);
});

test('an email can only have one account', async ({ page, browser }) => {
	const email = await signUp(page);
	const other = await browser.newPage();
	await other.goto('/signup');
	await other.getByRole('textbox', { name: 'Name' }).fill('Someone');
	await other.getByRole('textbox', { name: 'Email' }).fill(email);
	await other.locator('input[name=password]').fill(password);
	await other.getByRole('button', { name: 'Create account' }).click();
	await expect(other.getByText('An account with this email address already exists.')).toBeVisible();
});

test('signed out, the account page sends you to sign in and back', async ({ page }) => {
	const email = await signUp(page);
	await page.getByRole('button', { name: 'Sign out' }).first().click();
	await page.goto('/account');
	await expect(page).toHaveURL(/\/login\?next=%2Faccount$/);
	await signIn(page, email);
	await expect(page).toHaveURL(/\/account$/);
});

test('confirming the email address', async ({ page }) => {
	const email = await signUp(page);
	await page.goto(await linkFor(page, email, 'Confirm your email address'));
	await expect(page.getByText(`${email} is confirmed.`)).toBeVisible();
	await page.goto('/account');
	await expect(page.getByText('Confirmed', { exact: true })).toBeVisible();
});

test('resetting a forgotten password signs out other devices', async ({ page, browser }) => {
	const email = await signUp(page);
	const laptop = await browser.newPage();
	await laptop.goto('/forgot-password');
	await laptop.getByRole('textbox', { name: 'Email' }).fill(email);
	await laptop.getByRole('button', { name: 'Send link' }).click();
	await expect(laptop.getByText('a link to reset its password is on its way')).toBeVisible();

	await laptop.goto(await linkFor(laptop, email, 'Reset your password'));
	await laptop.locator('input[name=password]').fill('difference engine');
	await laptop.locator('input[name=confirm]').fill('difference engine');
	await laptop.getByRole('button', { name: 'Save password' }).click();
	await expect(laptop).toHaveURL(/\/account$/);

	await page.reload();
	await expect(page).toHaveURL(/\/login/);
	await signIn(page, email);
	await expect(page.getByText('Wrong email or password.')).toBeVisible();
	await signIn(page, email, 'difference engine');
	await expect(page).toHaveURL(/\/account$/);
});

test('an expired reset link says so', async ({ page }) => {
	await page.goto('/reset-password?token=nope');
	await page.locator('input[name=password]').fill('difference engine');
	await page.locator('input[name=confirm]').fill('difference engine');
	await page.getByRole('button', { name: 'Save password' }).click();
	await expect(page.getByText('This link is invalid or has expired.')).toBeVisible();
});

test('signing out another device', async ({ page, browser }) => {
	const email = await signUp(page);
	const phone = await browser.newPage();
	await signIn(phone, email);
	await expect(phone).toHaveURL(/\/account$/);

	await page.reload();
	const devices = page.locator('.tern-account', { hasText: 'Signed-in devices' });
	await expect(devices.getByRole('listitem')).toHaveCount(2);
	await expect(devices.getByRole('listitem').first()).toContainText('This device');
	await devices.getByRole('listitem').nth(1).getByRole('button', { name: 'Sign out' }).click();
	await expect(devices.getByRole('listitem')).toHaveCount(1);

	await phone.reload();
	await expect(phone).toHaveURL(/\/login/);
});

test('changing the password checks the current one', async ({ page }) => {
	await signUp(page);
	await page.getByLabel('Current password').fill('wrong');
	await page.getByLabel('New password').fill('difference engine');
	await page.getByRole('button', { name: 'Change password' }).click();
	await expect(page.getByText('That isn’t your current password.')).toBeVisible();
	await page.getByLabel('Current password').fill(password);
	await page.getByRole('button', { name: 'Change password' }).click();
	await expect(page.getByText('Password changed.')).toBeVisible();
});

test('passkeys: add one, then sign in with it', async ({ page, browser }) => {
	const cdp = await page.context().newCDPSession(page);
	await cdp.send('WebAuthn.enable');
	const { authenticatorId } = await cdp.send('WebAuthn.addVirtualAuthenticator', {
		options: { protocol: 'ctap2', transport: 'internal', hasResidentKey: true, hasUserVerification: true, isUserVerified: true, automaticPresenceSimulation: true }
	});
	const email = await signUp(page);
	const passkeys = page.locator('.tern-account', { hasText: 'Passkeys' });
	await passkeys.getByRole('button', { name: 'Add a passkey' }).click();
	await expect(passkeys.getByRole('listitem')).toHaveCount(1);
	await expect(passkeys.getByRole('listitem')).toContainText('never used');

	// Sign out, then back in with no email: the browser offers the passkey.
	await page.getByRole('button', { name: 'Sign out' }).first().click();
	await expect(page).toHaveURL(/\/login$/);
	await page.getByRole('button', { name: 'Sign in with a passkey' }).click();
	await expect(page).toHaveURL(/\/account$/);
	await expect(page.getByText(email)).toBeVisible();
	await expect(passkeys.getByRole('listitem')).toContainText('last used');

	await passkeys.getByRole('button', { name: 'Remove' }).click();
	await expect(passkeys.getByText('No passkeys yet.')).toBeVisible();
	await cdp.send('WebAuthn.removeVirtualAuthenticator', { authenticatorId });
	void browser;
});

test('deleting the account', async ({ page }) => {
	const email = await signUp(page);
	await page.getByRole('button', { name: 'Delete account' }).click();
	await page.getByRole('alertdialog').getByRole('button', { name: 'Delete account' }).click();
	await expect(page).toHaveURL(/\/$/);
	await signIn(page, email);
	await expect(page.getByText('Wrong email or password.')).toBeVisible();
});
