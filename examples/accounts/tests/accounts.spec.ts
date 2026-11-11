import AxeBuilder from '@axe-core/playwright';
import { expect, test, type Page } from '@playwright/test';
import { createHmac } from 'node:crypto';

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

function base32(s: string) {
	const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';
	let bits = '';
	for (const c of s.replace(/\s/g, '')) bits += alphabet.indexOf(c).toString(2).padStart(5, '0');
	const out = [];
	for (let i = 0; i + 8 <= bits.length; i += 8) out.push(parseInt(bits.slice(i, i + 8), 2));
	return Buffer.from(out);
}

function totp(secret: string, offset = 0) {
	const step = Math.floor(Date.now() / 30000) + offset;
	const msg = Buffer.alloc(8);
	msg.writeBigUInt64BE(BigInt(step));
	const h = createHmac('sha1', base32(secret)).update(msg).digest();
	const o = h[h.length - 1] & 0xf;
	return String((h.readUInt32BE(o) & 0x7fffffff) % 1000000).padStart(6, '0');
}

test('two-step sign-in with an authenticator app and recovery codes', async ({ page, browser }) => {
	const email = await signUp(page);
	const section = page.locator('.tern-account', { hasText: 'Two-step sign-in' });
	await section.getByRole('button', { name: 'Set up two-step sign-in' }).click();
	await expect(section.getByRole('img', { name: /QR code/ })).toBeVisible();
	await audit(page);
	const secret = (await section.getByTestId('two-step-secret').textContent())!;
	await section.locator('input[name=code]').fill(totp(secret));
	await section.getByRole('button', { name: 'Turn on' }).click();
	await expect(section.getByTestId('recovery-codes').locator('code')).toHaveCount(10);
	const codes = await section.getByTestId('recovery-codes').locator('code').allTextContents();
	await expect(section.getByText('On', { exact: true })).toBeVisible();

	const other = await browser.newPage();
	await signIn(other, email);
	await expect(other).toHaveURL(/\/login\/code\?next=%2Faccount$/);
	await other.locator('input[name=code]').fill('000000');
	await other.getByRole('button', { name: 'Continue' }).click(); // right after the code posted itself
	await expect(other.getByText('That code didn’t work.')).toBeVisible();
	await other.locator('input[name=code]').fill(totp(secret, 1));
	await expect(other).toHaveURL(/\/account$/);

	const third = await browser.newPage();
	await signIn(third, email);
	await third.getByRole('button', { name: 'Use a recovery code' }).click();
	await third.getByLabel('Recovery code').fill(codes[0]);
	await third.getByRole('button', { name: 'Continue' }).click();
	await expect(third).toHaveURL(/\/account$/);
	await expect(third.getByText('9 recovery codes left')).toBeVisible();
});

test('a code page without a password sign-in sends you to sign in', async ({ page }) => {
	await page.goto('/login/code');
	await expect(page).toHaveURL(/\/login$/);
});

test('changing the email address', async ({ page }) => {
	const email = await signUp(page);
	const next = newEmail();
	await page.getByLabel('New email address').fill(next);
	await page.getByRole('button', { name: 'Change email address' }).click();
	await expect(page.getByText('Check the new address')).toBeVisible();
	await page.goto(await linkFor(page, next, 'Confirm your new email address'));
	await expect(page.getByText(`${next} is confirmed.`)).toBeVisible();
	await page.getByRole('button', { name: 'Sign out' }).first().click();
	await signIn(page, email);
	await expect(page.getByText('Wrong email or password.')).toBeVisible();
	await signIn(page, next);
	await expect(page).toHaveURL(/\/account$/);
});
