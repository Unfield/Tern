import { getContext, setContext } from 'svelte';

/** What a Field tells the control inside it, so labels and messages are wired up. */
export interface FieldContext {
	readonly id: string;
	readonly describedBy: string | undefined;
	readonly invalid: boolean;
	readonly required: boolean;
	readonly disabled: boolean;
}

const key = Symbol('tern-field');

export function setField(ctx: FieldContext) {
	setContext(key, ctx);
}

export function getField(): FieldContext | undefined {
	return getContext<FieldContext | undefined>(key);
}
