// The server bundle for the compatibility suite: like an app's
// .tern/generated/server.js, but it renders one library case per request.
import '@ternkit/tern/polyfills';
import { render } from 'svelte/server';
import BitsUI from './cases/BitsUI.svelte';
import Dates from './cases/Dates.svelte';
import I18n from './cases/I18n.svelte';
import Ids from './cases/Ids.svelte';
import LayerChart from './cases/LayerChart.svelte';
import Locale from './cases/Locale.svelte';
import Lucide from './cases/Lucide.svelte';
import Markdown from './cases/Markdown.svelte';
import ModeWatcher from './cases/ModeWatcher.svelte';
import Paneforge from './cases/Paneforge.svelte';
import Runed from './cases/Runed.svelte';
import Sonner from './cases/Sonner.svelte';
import TanstackQuery from './cases/TanstackQuery.svelte';
import Utilities from './cases/Utilities.svelte';
import Validation from './cases/Validation.svelte';

/** Each case and text its HTML must contain. */
const cases = {
	'bits-ui': [BitsUI, ['Accordion content', 'Tab panel', 'Selected b', 'Open dialog', 'Hover me']],
	'@lucide/svelte': [Lucide, ['<svg', 'lucide-heart']],
	'svelte-sonner': [Sonner, ['toasts ready']],
	'mode-watcher': [ModeWatcher, ['mode:']],
	paneforge: [Paneforge, ['Left pane', 'Right pane', 'data-pane-group']],
	runed: [Runed, ['debounced: svelte persisted: 3']],
	'@tanstack/svelte-query': [TanstackQuery, ['user: Ada']],
	'date-fns, dayjs': [Dates, ['date-fns: 2026-11-10 3 days', 'dayjs: 2026-11-11']],
	'marked, highlight.js, katex': [Markdown, ['<h1>Title</h1>', '<strong>bold</strong>', 'hljs-keyword', 'class="katex"']],
	'nanoid, uuid, crypto': [Ids, ['nanoid 21 uuid 36 randomUUID 36']],
	'zod, valibot': [Validation, ['zod issues: 2 valibot ok: true']],
	'd3, floating-ui, clsx, tailwind-merge, lodash': [Utilities, ['<path d="M0,0L50,10L100,0"', 'class="p-4 text-red-500"', 'odd 2']],
	'svelte-i18n': [I18n, ['Hello Tern / 3 items']],
	layerchart: [LayerChart, ['height: 200px']],
	'toLocaleString': [Locale, ['5,000,000 5,000,000 $1,234.50 26%']]
};

globalThis.__tern_render = (payload) => {
	const p = JSON.parse(payload);
	if (p.list) {
		return JSON.stringify({ body: JSON.stringify(Object.entries(cases).map(([name, [, expect]]) => ({ name, expect }))) });
	}
	const [component] = cases[p.case];
	const out = render(component);
	return JSON.stringify({ head: out.head, body: out.body });
};
