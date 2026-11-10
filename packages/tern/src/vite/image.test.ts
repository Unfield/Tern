import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import sharp from 'sharp';
import { build, createServer } from 'vite';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import type { Picture } from '../image/types.js';
import { imagePlugin, pickWidths } from './image.js';

let root: string;

beforeAll(async () => {
	root = fs.mkdtempSync(path.join(os.tmpdir(), 'tern-image-'));
	await sharp({ create: { width: 1600, height: 900, channels: 3, background: '#3366cc' } })
		.jpeg()
		.toFile(path.join(root, 'photo.jpg'));
	await sharp({ create: { width: 300, height: 200, channels: 4, background: { r: 0, g: 0, b: 0, alpha: 0.5 } } })
		.png()
		.toFile(path.join(root, 'icon.png'));
	fs.writeFileSync(
		path.join(root, 'entry.js'),
		`import photo from './photo.jpg?image';\nimport small from './photo.jpg?w=400,800,4000&image';\nimport icon from './icon.png?image';\nexport { photo, small, icon };\n`
	);
});

afterAll(() => fs.rmSync(root, { recursive: true, force: true }));

async function buildOnce(ssr: boolean) {
	const outDir = path.join(root, ssr ? 'out-ssr' : 'out');
	await build({
		root,
		logLevel: 'silent',
		configFile: false,
		base: '/app/',
		plugins: [imagePlugin()],
		build: {
			outDir,
			ssr: ssr ? path.join(root, 'entry.js') : undefined,
			lib: ssr ? undefined : { entry: path.join(root, 'entry.js'), formats: ['es'], fileName: 'entry' }
		}
	});
	const file = fs.readdirSync(outDir).find((f) => f.endsWith('.js') || f.endsWith('.mjs'))!;
	const mod = (await import(path.join(outDir, file) + `?t=${Date.now()}`)) as Record<string, Picture>;
	return { outDir, mod };
}

describe('?image', () => {
	it('picks widths', () => {
		expect(pickWidths(1600, null)).toEqual([640, 1280, 1600]);
		expect(pickWidths(500, null)).toEqual([500]);
		expect(pickWidths(1600, '800,400,4000')).toEqual([400, 800, 1600]);
		expect(() => pickWidths(1600, 'x')).toThrow();
	});

	it('emits AVIF, WebP and fallback files in the client build, and the server build agrees', async () => {
		const client = await buildOnce(false);
		const { photo, small, icon } = client.mod;

		expect(photo!.width).toBe(1600);
		expect(photo!.height).toBe(900);
		expect(photo!.src).toMatch(/^\/app\/_app\/immutable\/img\/photo-1600-[0-9a-f]{10}\.jpeg$/);
		expect(photo!.sources.map((s) => s.type)).toEqual(['image/avif', 'image/webp']);
		expect(photo!.srcset.split(', ').map((s) => s.split(' ')[1])).toEqual(['640w', '1280w', '1600w']);
		expect(small!.srcset.split(', ').map((s) => s.split(' ')[1])).toEqual(['400w', '800w', '1600w']);
		expect(icon!.src).toMatch(/\.png$/); // transparency keeps PNG as the fallback

		for (const url of [photo!.src, ...photo!.sources.flatMap((s) => s.srcset.split(', ').map((x) => x.split(' ')[0]!))]) {
			const file = path.join(client.outDir, url.replace('/app/', ''));
			expect(fs.existsSync(file), url).toBe(true);
		}
		const avif = photo!.sources[0]!.srcset.split(', ')[0]!.split(' ')[0]!;
		const meta = await sharp(path.join(client.outDir, avif.replace('/app/', ''))).metadata();
		expect(meta.width).toBe(640);
		expect(meta.format).toBe('heif');

		const server = await buildOnce(true);
		expect(server.mod.photo).toEqual(photo);
		expect(fs.existsSync(path.join(server.outDir, '_app'))).toBe(false);
	}, 60_000);

	it('serves images on demand in development', async () => {
		const server = await createServer({ root, logLevel: 'silent', configFile: false, plugins: [imagePlugin()], server: { port: 0, host: '127.0.0.1' } });
		await server.listen();
		try {
			const mod = (await server.ssrLoadModule(path.join(root, 'entry.js'))) as Record<string, Picture>;
			const url = mod.photo!.sources[1]!.srcset.split(' ')[0]!;
			expect(url).toMatch(/^\/@tern-image\/photo-640-[0-9a-f]{10}\.webp$/);
			const address = server.httpServer!.address() as { port: number };
			const res = await fetch(`http://127.0.0.1:${address.port}${url}`);
			expect(res.status).toBe(200);
			expect(res.headers.get('content-type')).toBe('image/webp');
			expect((await sharp(Buffer.from(await res.arrayBuffer())).metadata()).width).toBe(640);
		} finally {
			await server.close();
		}
	}, 60_000);
});
