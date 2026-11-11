// Passkeys in the browser, for the accounts package's endpoints. Each
// helper asks the server for options, hands them to the browser's
// WebAuthn API, and posts the authenticator's answer back.

import { getBase } from './base.js';

export type PasskeyOptions = {
	/** Where the accounts package mounts its passkey endpoints (Options.PasskeyPath). */
	path?: string;
};

/** A passkey ceremony that didn't finish. cancelled is true when the user closed the browser's prompt. */
export class PasskeyError extends Error {
	cancelled: boolean;
	constructor(message: string, cancelled = false) {
		super(message);
		this.name = 'PasskeyError';
		this.cancelled = cancelled;
	}
}

/** Whether this browser can use passkeys. */
export function passkeysSupported(): boolean {
	return typeof window !== 'undefined' && typeof window.PublicKeyCredential === 'function' && !!navigator.credentials;
}

export function toBase64url(buf: ArrayBuffer | Uint8Array): string {
	const bytes = buf instanceof Uint8Array ? buf : new Uint8Array(buf);
	let s = '';
	for (const b of bytes) s += String.fromCharCode(b);
	return btoa(s).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}

export function fromBase64url(s: string): Uint8Array<ArrayBuffer> {
	const b = atob(s.replace(/-/g, '+').replace(/_/g, '/') + '='.repeat((4 - (s.length % 4)) % 4));
	const out = new Uint8Array(new ArrayBuffer(b.length));
	for (let i = 0; i < b.length; i++) out[i] = b.charCodeAt(i);
	return out;
}

const endpoint = (o: PasskeyOptions | undefined, p: string) => getBase() + (o?.path ?? '/_tern/passkeys').replace(/\/$/, '') + p;

async function post(url: string, body?: unknown): Promise<Record<string, unknown>> {
	const res = await fetch(url, {
		method: 'POST',
		headers: { 'content-type': 'application/json', accept: 'application/json' },
		credentials: 'same-origin',
		body: JSON.stringify(body ?? {})
	});
	const data = (await res.json().catch(() => ({}))) as Record<string, unknown>;
	if (!res.ok) throw new PasskeyError(typeof data.message === 'string' ? data.message : `passkey request failed (${res.status})`);
	return data;
}

function failed(e: unknown): never {
	if (e instanceof PasskeyError) throw e;
	const name = (e as { name?: string })?.name;
	// NotAllowedError is the browser's answer for both "cancelled" and "timed out".
	if (name === 'NotAllowedError' || name === 'AbortError') throw new PasskeyError('cancelled', true);
	throw new PasskeyError((e as Error)?.message ?? String(e));
}

type Descriptor = { type: 'public-key'; id: string };

/** Adds a passkey to the signed-in user's account. name labels it in their list; the default names the browser and system. */
export async function registerPasskey(options?: PasskeyOptions & { name?: string }): Promise<void> {
	try {
		const o = (await post(endpoint(options, '/register/options'))) as Record<string, any>;
		const publicKey: PublicKeyCredentialCreationOptions = {
			...o,
			challenge: fromBase64url(o.challenge),
			user: { ...o.user, id: fromBase64url(o.user.id) },
			excludeCredentials: ((o.excludeCredentials ?? []) as Descriptor[]).map((c) => ({ type: c.type, id: fromBase64url(c.id) }))
		} as unknown as PublicKeyCredentialCreationOptions;
		const cred = (await navigator.credentials.create({ publicKey })) as PublicKeyCredential | null;
		if (!cred) throw new PasskeyError('cancelled', true);
		const r = cred.response as AuthenticatorAttestationResponse;
		await post(endpoint(options, '/register'), {
			id: cred.id,
			type: cred.type,
			name: options?.name ?? '',
			response: { clientDataJSON: toBase64url(r.clientDataJSON), attestationObject: toBase64url(r.attestationObject) }
		});
	} catch (e) {
		failed(e);
	}
}

/** Signs in with a passkey. The browser offers the user's passkeys for this site; no email is needed. */
export async function signInWithPasskey(options?: PasskeyOptions & { remember?: boolean }): Promise<void> {
	try {
		const o = (await post(endpoint(options, '/signin/options'))) as Record<string, any>;
		const publicKey: PublicKeyCredentialRequestOptions = {
			...o,
			challenge: fromBase64url(o.challenge),
			allowCredentials: []
		} as unknown as PublicKeyCredentialRequestOptions;
		const cred = (await navigator.credentials.get({ publicKey })) as PublicKeyCredential | null;
		if (!cred) throw new PasskeyError('cancelled', true);
		const r = cred.response as AuthenticatorAssertionResponse;
		await post(endpoint(options, '/signin'), {
			id: cred.id,
			type: cred.type,
			remember: !!options?.remember,
			response: {
				clientDataJSON: toBase64url(r.clientDataJSON),
				authenticatorData: toBase64url(r.authenticatorData),
				signature: toBase64url(r.signature),
				userHandle: r.userHandle ? toBase64url(r.userHandle) : ''
			}
		});
	} catch (e) {
		failed(e);
	}
}
