import type { Messages } from '../index.js';
import { builtin } from './builtin.js';
import { format } from './format.js';

/** A message key from src/locales, or any string when the app has none. */
export type MessageKey = [keyof Messages] extends [never] ? string : keyof Messages & string;

type Flat = Record<string, string>;
type Nested = { [key: string]: string | Nested };

export interface I18nConfig {
	defaultLocale: string;
	locales?: string[];
	/** Client: lazy loaders. */
	load?: Record<string, () => Promise<{ default: Nested }>>;
	/** Server: every locale's messages, eagerly imported. */
	messages?: Record<string, Nested>;
}

class I18nState {
	locale = $state('');
	messages = $state<Flat>({});
	fallback: Flat = {};
}

const state = new I18nState();
let config: I18nConfig | null = null;
const loaded = new Map<string, Flat>();
const warned = new Set<string>();

function flatten(tree: Nested, prefix = '', out: Flat = {}): Flat {
	for (const [k, v] of Object.entries(tree)) {
		const key = prefix ? `${prefix}.${k}` : k;
		if (typeof v === 'string') out[key] = v;
		else flatten(v, key, out);
	}
	return out;
}

export function configureI18n(c: I18nConfig | null) {
	config = c;
	if (c?.messages) {
		for (const [l, m] of Object.entries(c.messages)) loaded.set(l, flatten(m));
	}
}

async function messagesFor(locale: string): Promise<Flat> {
	const hit = loaded.get(locale);
	if (hit) return hit;
	const loader = config?.load?.[locale];
	if (!loader) return {};
	const flat = flatten((await loader()).default);
	loaded.set(locale, flat);
	return flat;
}

/** Loads a locale's messages (and the default's, for fallbacks) and makes it current. */
export async function useLocale(locale: string | undefined): Promise<void> {
	if (!config || !locale) return;
	const [messages, fallback] = await Promise.all([messagesFor(locale), messagesFor(config.defaultLocale)]);
	state.fallback = fallback;
	state.messages = messages;
	state.locale = locale;
	if (typeof document !== 'undefined') document.documentElement.lang = locale;
}

/** Synchronous version for the server renderer, where every locale is loaded. */
export function useLocaleSync(locale: string | undefined) {
	if (!config || !locale) return;
	state.fallback = loaded.get(config.defaultLocale) ?? {};
	state.messages = loaded.get(locale) ?? {};
	state.locale = locale;
}

/**
 * Translates a message from src/locales into the current locale:
 *
 * ```svelte
 * <h1>{t('welcome', { name: data.user })}</h1>
 * <p>{t('todos.left', { count: data.left })}</p>
 * ```
 *
 * Messages use ICU syntax for placeholders, plurals and selects. A message
 * missing from the current locale falls back to the default locale, and
 * then to the key.
 */
export function t(key: MessageKey, params?: Record<string, unknown>): string {
	const message = state.messages[key] ?? state.fallback[key];
	if (message === undefined) {
		if (import.meta.env?.DEV && !warned.has(key)) {
			warned.add(key);
			console.warn(`[tern] missing message "${key}"`);
		}
		return key;
	}
	return format(state.locale || config?.defaultLocale || 'en', message, params);
}

/**
 * Translates one of Tern's own texts, such as Tern UI's labels and the
 * validation messages: the app's message for key in src/locales if it has
 * one, else Tern's English. Their keys start with "tern.".
 */
export function tb(key: string, params?: Record<string, unknown>): string {
	const message = state.messages[key] ?? state.fallback[key] ?? builtin[key] ?? key;
	return format(state.locale || config?.defaultLocale || 'en', message, params);
}

/** The current locale and the ones the app supports. */
export const i18n = {
	/** The current locale, such as "de", or "" if the app has no src/locales. */
	get locale(): string {
		return state.locale;
	},
	/** Every locale in src/locales. */
	get locales(): string[] {
		return config?.locales ?? Object.keys(config?.messages ?? {});
	},
	/** The locale used for missing messages. */
	get defaultLocale(): string {
		return config?.defaultLocale ?? '';
	}
};

/**
 * Switches the user to another locale: stores it in the locale cookie, which
 * the server prefers over Accept-Language, and reloads the page's data. For
 * locales in the URL (a lang route parameter), link to the other URL instead.
 */
export async function setLocale(locale: string, base = ''): Promise<void> {
	document.cookie = `locale=${encodeURIComponent(locale)}; path=${base || '/'}; max-age=31536000; samesite=lax`;
	const { invalidateAll } = await import('./router.svelte.js');
	await invalidateAll();
}
