<script lang="ts">
	import { computePosition } from '@floating-ui/dom';
	import { scaleLinear } from 'd3-scale';
	import { line } from 'd3-shape';
	import { clsx } from 'clsx';
	import { twMerge } from 'tailwind-merge';
	import { debounce, groupBy } from 'lodash-es';
	const x = scaleLinear().domain([0, 10]).range([0, 100]);
	const path = line<[number, number]>()([[0, 0], [x(5), 10], [x(10), 0]]);
	const cls = twMerge(clsx('p-2', { 'p-4': true }), 'text-red-500');
	const groups = groupBy([1, 2, 3, 4], (n) => (n % 2 ? 'odd' : 'even'));
	debounce(() => {}, 10)();
	void computePosition;
</script>

<svg><path d={path} /></svg>
<p class={cls}>odd {groups.odd?.length}</p>
