import type { Component } from 'svelte';
import { render } from 'svelte/server';

/** An email component module from src/emails. */
export interface EmailModule {
	default: Component<any>;
	/** Props for the preview page under tern dev. */
	preview?: Record<string, unknown>;
}

export interface EmailResult {
	subject: string;
	html: string;
	error?: string;
}

const entities: Record<string, string> = { '&amp;': '&', '&lt;': '<', '&gt;': '>', '&quot;': '"', '&#39;': "'", '&#x27;': "'" };

function decode(s: string): string {
	return s.replace(/&(amp|lt|gt|quot|#39|#x27);/g, (m) => entities[m] ?? m);
}

/**
 * Creates the function Go calls to render emails: a component's markup
 * becomes the body, its <svelte:head> the head, and its <title> the
 * subject.
 */
export function createEmailRenderer(emails: Record<string, EmailModule>) {
	return (name: string, propsJSON: string): string => {
		if (name === '') {
			// The list of emails and their preview props, for the dev preview.
			return JSON.stringify(Object.fromEntries(Object.entries(emails).map(([n, m]) => [n, m.preview ?? {}])));
		}
		const mod = emails[name];
		if (!mod) return JSON.stringify({ subject: '', html: '', error: `no email named ${name} in src/emails` } satisfies EmailResult);
		const props = JSON.parse(propsJSON || '{}') as Record<string, unknown>;
		const out = render(mod.default, { props });
		const title = /<title>([\s\S]*?)<\/title>/.exec(out.head);
		const subject = title ? decode(title[1]!.trim()) : '';
		const head = out.head.replace(/<!--[\s\S]*?-->/g, '');
		const body = out.body.replace(/<!--[\s\S]*?-->/g, '');
		const html =
			'<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">' +
			'<meta name="color-scheme" content="light dark">' +
			head +
			'</head><body>' +
			body +
			'</body></html>';
		return JSON.stringify({ subject, html } satisfies EmailResult);
	};
}
