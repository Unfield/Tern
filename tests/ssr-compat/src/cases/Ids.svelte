<script lang="ts">
	import { nanoid } from 'nanoid';
	import { v4 } from 'uuid';
	const a = nanoid();
	const b = v4();
	const c = crypto.randomUUID();
</script>

<p>nanoid {a.length} uuid {b.length} randomUUID {c.length}</p>
