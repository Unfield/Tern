import type { ActionReturn } from 'svelte/action';
import { applyAction, invalidateAll } from './router.svelte.js';
import type { ActionResult } from './types.js';

export interface SubmitInput {
	action: URL;
	formData: FormData;
	formElement: HTMLFormElement;
	controller: AbortController;
	submitter: HTMLElement | null;
	/** Stops the submission. */
	cancel(): void;
}

export interface SubmitResult<Success, Failure> {
	action: URL;
	formData: FormData;
	formElement: HTMLFormElement;
	result: ActionResult<Success, Failure>;
	/**
	 * Does what `use:enhance` does by default: reset the form and rerun
	 * loads after a success, then apply the result to the page.
	 */
	update(opts?: { reset?: boolean; invalidateAll?: boolean }): Promise<void>;
}

/**
 * Runs before a form is submitted. Return a function to handle the
 * result yourself; call `update()` in it to keep the default behavior.
 */
export type SubmitFunction<Success = Record<string, unknown>, Failure = Record<string, unknown>> = (
	input: SubmitInput
) =>
	| void
	| ((result: SubmitResult<Success, Failure>) => void | Promise<void>)
	| Promise<void | ((result: SubmitResult<Success, Failure>) => void | Promise<void>)>;

// Cloning a form without children reads its own action/method/enctype,
// even if it contains inputs named "action" or "method".
function attrs(form: HTMLFormElement) {
	return HTMLFormElement.prototype.cloneNode.call(form) as HTMLFormElement;
}

/**
 * Progressively enhances a form that posts to a page action. Without
 * JavaScript the form still works; with it, the page updates in place.
 *
 * ```svelte
 * <form method="POST" action="?/create" use:enhance>
 * ```
 */
export function enhance<Success = Record<string, unknown>, Failure = Record<string, unknown>>(
	form: HTMLFormElement,
	submit: SubmitFunction<Success, Failure> = () => {}
): ActionReturn<SubmitFunction<Success, Failure> | undefined> {
	async function defaultUpdate(
		action: URL,
		formElement: HTMLFormElement,
		result: ActionResult<Success, Failure>,
		{ reset = true, invalidateAll: invalidate = true }: { reset?: boolean; invalidateAll?: boolean } = {}
	) {
		if (result.type === 'success') {
			if (reset) HTMLFormElement.prototype.reset.call(formElement);
			if (invalidate) await invalidateAll();
		}
		const samePage = location.origin + location.pathname === action.origin + action.pathname;
		if (samePage || result.type === 'redirect' || result.type === 'error') {
			await applyAction(result as ActionResult);
		}
	}

	async function onSubmit(event: SubmitEvent) {
		const submitter = event.submitter as HTMLButtonElement | HTMLInputElement | null;
		const own = attrs(form);
		const method = submitter?.hasAttribute('formmethod') ? submitter.formMethod : own.method;
		if (method !== 'post') return;
		event.preventDefault();

		const action = new URL(submitter?.hasAttribute('formaction') ? submitter.formAction : own.action);
		const enctype = submitter?.hasAttribute('formenctype') ? submitter.formEnctype : own.enctype;
		const formData = new FormData(form, submitter);
		const controller = new AbortController();

		let cancelled = false;
		const callback = await submit({
			action,
			formData,
			formElement: form,
			controller,
			submitter,
			cancel: () => (cancelled = true)
		});
		if (cancelled) return;

		let result: ActionResult<Success, Failure>;
		try {
			const res = await fetch(action, {
				method: 'POST',
				headers: { accept: 'application/json', 'x-tern-action': 'true' },
				body:
					enctype === 'multipart/form-data'
						? formData
						: new URLSearchParams(formData as unknown as Record<string, string>),
				cache: 'no-store',
				credentials: 'same-origin',
				signal: controller.signal
			});
			if (res.headers.get('content-type')?.includes('application/json')) {
				result = await res.json();
			} else {
				result = { type: 'error', status: res.status, error: { message: res.statusText || 'Request failed' } };
			}
		} catch (err) {
			if (controller.signal.aborted) return;
			result = { type: 'error', status: 500, error: { message: err instanceof Error ? err.message : String(err) } };
		}

		const update = (opts?: { reset?: boolean; invalidateAll?: boolean }) =>
			defaultUpdate(action, form, result, opts);
		if (callback) {
			await callback({ action, formData, formElement: form, result, update });
		} else {
			await update();
		}
	}

	form.addEventListener('submit', onSubmit);
	return {
		update(next) {
			submit = next ?? (() => {});
		},
		destroy() {
			form.removeEventListener('submit', onSubmit);
		}
	};
}
