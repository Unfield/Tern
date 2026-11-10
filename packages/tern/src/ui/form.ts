import { getContext, setContext } from 'svelte';
import type { FormField } from '../runtime/validate.js';

/** What a Form tells the Fields and controls inside it. */
export interface FormContext {
	/** The message to show for a field, from the browser's checks or the server. */
	error(name: string): string | undefined;
	/** What was posted before, to refill a field after a failure without JavaScript. */
	value(name: string): string | undefined;
	/** A field's rules, such as whether it's required. */
	field(name: string): FormField | undefined;
}

const key = Symbol('tern-form');

export function setForm(ctx: FormContext) {
	setContext(key, ctx);
}

export function getForm(): FormContext | undefined {
	return getContext<FormContext | undefined>(key);
}
