import { applyAction, invalidateAll } from './router.svelte.js';
// Cloning a form without children reads its own action/method/enctype,
// even if it contains inputs named "action" or "method".
function attrs(form) {
    return HTMLFormElement.prototype.cloneNode.call(form);
}
/**
 * Progressively enhances a form that posts to a page action. Without
 * JavaScript the form still works; with it, the page updates in place.
 *
 * ```svelte
 * <form method="POST" action="?/create" use:enhance>
 * ```
 */
export function enhance(form, submit = () => { }) {
    async function defaultUpdate(action, formElement, result, { reset = true, invalidateAll: invalidate = true } = {}) {
        if (result.type === 'success') {
            if (reset)
                HTMLFormElement.prototype.reset.call(formElement);
            if (invalidate)
                await invalidateAll();
        }
        const samePage = location.origin + location.pathname === action.origin + action.pathname;
        if (samePage || result.type === 'redirect' || result.type === 'error') {
            await applyAction(result);
        }
    }
    async function onSubmit(event) {
        const submitter = event.submitter;
        const own = attrs(form);
        const method = submitter?.hasAttribute('formmethod') ? submitter.formMethod : own.method;
        if (method !== 'post')
            return;
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
        if (cancelled)
            return;
        let result;
        try {
            const res = await fetch(action, {
                method: 'POST',
                headers: { accept: 'application/json', 'x-tern-action': 'true' },
                body: enctype === 'multipart/form-data'
                    ? formData
                    : new URLSearchParams(formData),
                cache: 'no-store',
                credentials: 'same-origin',
                signal: controller.signal
            });
            if (res.headers.get('content-type')?.includes('application/json')) {
                result = await res.json();
            }
            else {
                result = { type: 'error', status: res.status, error: { message: res.statusText || 'Request failed' } };
            }
        }
        catch (err) {
            if (controller.signal.aborted)
                return;
            result = { type: 'error', status: 500, error: { message: err instanceof Error ? err.message : String(err) } };
        }
        const update = (opts) => defaultUpdate(action, form, result, opts);
        if (callback) {
            await callback({ action, formData, formElement: form, result, update });
        }
        else {
            await update();
        }
    }
    form.addEventListener('submit', onSubmit);
    return {
        update(next) {
            submit = next ?? (() => { });
        },
        destroy() {
            form.removeEventListener('submit', onSubmit);
        }
    };
}
