import { describe, expect, it } from 'vitest';
import { DeferredScope, readStreamed, revive } from './deferred.js';

describe('revive', () => {
	it('replaces placeholders deep in the data, and nothing else', () => {
		const seen: number[] = [];
		const out = revive({ a: { __tern_deferred: 1 }, b: [{ __tern_deferred: 2 }], c: { __tern_deferred: 3, other: true } }, (id) => {
			seen.push(id);
			return Promise.resolve(id);
		});
		expect(seen).toEqual([1, 2]);
		expect(out.c).toEqual({ __tern_deferred: 3, other: true });
	});
});

describe('DeferredScope', () => {
	it('resolves values that arrive before or after they are requested', async () => {
		const scope = new DeferredScope();
		scope.settle({ id: 1, data: 'early' });
		const late = scope.promise(2);
		scope.settle({ id: 2, data: 'late' });
		await expect(scope.promise(1)).resolves.toBe('early');
		await expect(late).resolves.toBe('late');
	});

	it('rejects errors and values still pending when closed', async () => {
		const scope = new DeferredScope();
		const failed = scope.promise(1);
		const pending = scope.promise(2);
		scope.settle({ id: 1, error: { message: 'nope' } });
		scope.close('gone');
		await expect(failed).rejects.toThrow('nope');
		await expect(pending).rejects.toThrow('gone');
	});
});

describe('readStreamed', () => {
	it('returns the payload before the deferred values finish streaming', async () => {
		const enc = new TextEncoder();
		let push!: (s: string) => void;
		let end!: () => void;
		const body = new ReadableStream<Uint8Array>({
			start(controller) {
				push = (s) => controller.enqueue(enc.encode(s));
				end = () => controller.close();
			}
		});
		push(JSON.stringify({ type: 'data', status: 200, nodes: [{ slow: { __tern_deferred: 1 } }] }) + '\n');
		const payload = await readStreamed<{ nodes: [{ slow: Promise<string> }] }>(new Response(body));
		const slow = payload.nodes[0].slow;
		push('{"id":1,"data":"done"}\n');
		end();
		await expect(slow).resolves.toBe('done');
	});
});
