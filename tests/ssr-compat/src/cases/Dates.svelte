<script lang="ts">
	import { format, formatDistanceStrict } from 'date-fns';
	import dayjs from 'dayjs';
	const d = new Date(Date.UTC(2026, 10, 10, 12, 0, 0));
</script>

<p>date-fns: {format(d, 'yyyy-MM-dd')} {formatDistanceStrict(d, new Date(Date.UTC(2026, 10, 13, 12)))}</p>
<p>dayjs: {dayjs(d).add(1, 'day').format('YYYY-MM-DD')}</p>
