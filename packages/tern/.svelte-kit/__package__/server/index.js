import { render } from 'svelte/server';
import DefaultError from '../runtime/DefaultError.svelte';
import Passthrough from '../runtime/Passthrough.svelte';
import Root from '../runtime/Root.svelte';
import { revive } from '../runtime/deferred.js';
import { plan } from '../runtime/select.js';
import { applyPayloadToPage } from '../runtime/state.svelte.js';
/**
 * Creates the function the Go server calls to render a page. It takes the
 * same payload the client receives, so the server and the client render
 * exactly the same tree and hydration matches.
 */
export function createRenderer(manifest) {
    const layout = (i) => manifest.layouts[i]?.component ?? Passthrough;
    return (payloadJSON, href) => {
        const p = JSON.parse(payloadJSON);
        // Deferred values are still loading: render their pending state.
        p.nodes = revive(p.nodes, () => new Promise(() => { }));
        const what = plan(manifest, p);
        let components;
        switch (what.kind) {
            case 'not-a-page':
                return JSON.stringify({ skip: true });
            case 'page':
                components = [...what.layouts.map(layout), manifest.routes[what.route].page];
                break;
            case 'error': {
                const errors = what.route ? manifest.routes[what.route].errors : manifest.fallback.errors;
                components = [...what.layouts.map(layout), errors[what.errorIndex].component];
                break;
            }
            case 'default-error':
                components = [DefaultError];
        }
        applyPayloadToPage(p, new URL(href), {}, what.data);
        const form = p.hasForm ? (p.form ?? null) : null;
        const out = render(Root, { props: { components, data: what.data, form } });
        return JSON.stringify({ head: out.head, body: out.body });
    };
}
