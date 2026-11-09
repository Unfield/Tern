import type { ErrorEntry, Payload } from './types.js';
/** The parts of a manifest that decide what renders. */
export interface Tree<C> {
    routes: Record<string, {
        layouts: number[];
        errors: {
            layouts: number;
        }[];
    } & C>;
    fallback: {
        layouts: number[];
        errors: {
            layouts: number;
        }[];
    };
}
/** What to render for a payload. Shared by the server and the client so hydration matches. */
export type Plan = {
    kind: 'page';
    route: string;
    layouts: number[];
    data: Record<string, unknown>[];
} | {
    kind: 'error';
    route: string | null;
    layouts: number[];
    errorIndex: number;
    data: Record<string, unknown>[];
} | {
    kind: 'default-error';
    data: Record<string, unknown>[];
} | {
    kind: 'not-a-page';
};
export declare function mergeData(list: (Record<string, unknown> | null | undefined)[]): Record<string, unknown>[];
/** The innermost error page whose layouts all loaded successfully. */
export declare function pickErrorPage(errors: Pick<ErrorEntry, 'layouts'>[], failed: number): number;
export declare function plan<C>(tree: Tree<C>, p: Payload): Plan;
