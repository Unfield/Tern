export function mergeData(list) {
    const out = [];
    let acc = {};
    for (const node of list) {
        acc = { ...acc, ...(node ?? {}) };
        out.push(acc);
    }
    return out;
}
/** The innermost error page whose layouts all loaded successfully. */
export function pickErrorPage(errors, failed) {
    let pick = -1;
    errors.forEach((e, i) => {
        if (e.layouts <= failed)
            pick = i;
    });
    return pick;
}
export function plan(tree, p) {
    const received = p.nodes ?? [];
    const route = p.route ? tree.routes[p.route] : undefined;
    if (p.type === 'data') {
        if (!route || !p.route)
            return { kind: 'not-a-page' };
        return { kind: 'page', route: p.route, layouts: route.layouts, data: mergeData(received) };
    }
    if (p.type !== 'error' || (p.route && !route)) {
        return { kind: 'not-a-page' }; // an endpoint, or a non-render result
    }
    const layouts = route ? route.layouts : tree.fallback.layouts;
    const errors = route ? route.errors : tree.fallback.errors;
    const index = pickErrorPage(errors, p.failed ?? 0);
    if (index < 0) {
        return { kind: 'default-error', data: [{}] };
    }
    const above = errors[index].layouts;
    const merged = mergeData(received.slice(0, above));
    return {
        kind: 'error',
        route: p.route ?? null,
        layouts: layouts.slice(0, above),
        errorIndex: index,
        data: [...merged, merged[merged.length - 1] ?? {}]
    };
}
