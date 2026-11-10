import { getContext, setContext } from 'svelte';

/** What a Field tells the control inside it, so labels and messages are wired up. */
export interface FieldContext {
	readonly id: string;
	/** The label's id, for controls that aren't labelable elements and need aria-labelledby. */
	readonly labelId: string;
	readonly describedBy: string | undefined;
	readonly invalid: boolean;
	readonly required: boolean;
	readonly disabled: boolean;
	/** Custom controls call this: the label then focuses the element with the field's id instead of using for. */
	custom(): void;
	/** The name the control posts under, from the Field. */
	readonly name: string | undefined;
	/** What was posted before, to refill the control after a failed post. */
	readonly value: string | undefined;
}

const key = Symbol('tern-field');

export function setField(ctx: FieldContext) {
	setContext(key, ctx);
}

export function getField(): FieldContext | undefined {
	return getContext<FieldContext | undefined>(key);
}
