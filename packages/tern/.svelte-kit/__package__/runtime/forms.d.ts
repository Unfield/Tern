import type { ActionReturn } from 'svelte/action';
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
    update(opts?: {
        reset?: boolean;
        invalidateAll?: boolean;
    }): Promise<void>;
}
/**
 * Runs before a form is submitted. Return a function to handle the
 * result yourself; call `update()` in it to keep the default behavior.
 */
export type SubmitFunction<Success = Record<string, unknown>, Failure = Record<string, unknown>> = (input: SubmitInput) => void | ((result: SubmitResult<Success, Failure>) => void | Promise<void>) | Promise<void | ((result: SubmitResult<Success, Failure>) => void | Promise<void>)>;
/**
 * Progressively enhances a form that posts to a page action. Without
 * JavaScript the form still works; with it, the page updates in place.
 *
 * ```svelte
 * <form method="POST" action="?/create" use:enhance>
 * ```
 */
export declare function enhance<Success = Record<string, unknown>, Failure = Record<string, unknown>>(form: HTMLFormElement, submit?: SubmitFunction<Success, Failure>): ActionReturn<SubmitFunction<Success, Failure> | undefined>;
