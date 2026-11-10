<script lang="ts">
	import * as v from 'valibot';
	import { z } from 'zod';
	const zs = z.object({ email: z.string().email(), age: z.number().min(18) });
	const zr = zs.safeParse({ email: 'nope', age: 3 });
	const vs = v.object({ email: v.pipe(v.string(), v.email()) });
	const vr = v.safeParse(vs, { email: 'a@b.co' });
</script>

<p>zod issues: {zr.success ? 0 : zr.error.issues.length} valibot ok: {vr.success}</p>
