import { describe, expect, it } from 'vitest';
import { ApiError, createStream } from './api.js';

const route = (pattern: string) => pattern;

// A fetch that answers with these chunks of an event stream, split anywhere.
function serve(chunks: string[], init: ResponseInit = {}, seen?: { url?: string; init?: RequestInit; aborted?: boolean }) {
	return async (url: RequestInfo | URL, req?: RequestInit) => {
		if (seen) Object.assign(seen, { url: String(url), init: req });
		req?.signal?.addEventListener('abort', () => seen && (seen.aborted = true));
		const enc = new TextEncoder();
		const body = new ReadableStream<Uint8Array>({
			async pull(c) {
				const next = chunks.shift();
				if (next === undefined) return c.close();
				c.enqueue(enc.encode(next));
			}
		});
		return new Response(body, { status: 200, headers: { 'content-type': 'text/event-stream' }, ...init });
	};
}

async function all<T>(s: AsyncIterable<T>): Promise<T[]> {
	const out: T[] = [];
	for await (const v of s) out.push(v);
	return out;
}

describe('stream', () => {
	const stream = createStream(route);

	it('yields each value, across chunks, until done', async () => {
		const seen: { url?: string; init?: RequestInit } = {};
		const fetch = serve(['data: {"text":"Hel', 'lo"}\n\ndata: {"text":" there"}\r\n\r\n', 'event: done\ndata: {}\n\n'], {}, seen);
		expect(await all(stream('POST /api/ask', { body: { question: 'hi' }, fetch }))).toEqual([{ text: 'Hello' }, { text: ' there' }]);
		expect(seen.init?.method).toBe('POST');
		expect(seen.init?.body).toBe('{"question":"hi"}');
		expect((seen.init?.headers as Record<string, string>).Accept).toBe('text/event-stream');
	});

	it('throws the error the server ends with, after its values', async () => {
		const fetch = serve(['data: 1\n\n', 'event: error\ndata: {"status":429,"message":"Slow down."}\n\n']);
		const got: unknown[] = [];
		const err = await (async () => {
			try {
				for await (const v of stream('GET /api/ask', { fetch })) got.push(v);
			} catch (e) {
				return e;
			}
		})();
		expect(got).toEqual([1]);
		expect(err).toBeInstanceOf(ApiError);
		expect((err as ApiError).status).toBe(429);
		expect((err as ApiError).message).toBe('Slow down.');
	});

	it('throws a failed request as an ApiError', async () => {
		const fetch = async () => new Response('{"message":"Sign in first."}', { status: 401, headers: { 'content-type': 'application/json' } });
		await expect(all(stream('GET /api/ask', { fetch }))).rejects.toMatchObject({ status: 401, message: 'Sign in first.' });
	});

	it('notices a stream that was cut off', async () => {
		await expect(all(stream('GET /api/ask', { fetch: serve(['data: 1\n\n']) }))).rejects.toMatchObject({ message: 'The stream ended before it was done' });
	});

	it('stops the request when the loop stops early', async () => {
		const seen: { aborted?: boolean } = {};
		const fetch = serve(['data: 1\n\n', 'data: 2\n\n', 'data: 3\n\n', 'event: done\ndata: {}\n\n'], {}, seen);
		for await (const v of stream('GET /api/ask', { fetch })) {
			if (v === 1) break;
		}
		expect(seen.aborted).toBe(true);
	});

	it('ends quietly when aborted', async () => {
		const fetch = serve(['data: 1\n\n', 'data: 2\n\n', 'event: done\ndata: {}\n\n']);
		const s = stream('GET /api/ask', { fetch });
		const got: unknown[] = [];
		for await (const v of s) {
			got.push(v);
			s.abort();
		}
		expect(got).toEqual([1]);
	});
});
