import type { Component } from 'svelte';
interface Props {
    /** Layout components, outermost first, then the page or error component. */
    components: Component<any>[];
    /** Data for each component, already merged with its parents' data. */
    data: Record<string, unknown>[];
    form: unknown;
}
declare const Root: Component<Props, {}, "">;
type Root = ReturnType<typeof Root>;
export default Root;
