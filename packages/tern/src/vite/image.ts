import { createHash } from 'node:crypto';
import fs from 'node:fs';
import path from 'node:path';
import type { Plugin, ResolvedConfig } from 'vite';
import type { Picture } from '../image/types.js';

/**
 * Responsive images: `import hero from './hero.jpg?image'` resizes the image
 * to several widths in AVIF, WebP and its own format, and gives a Picture
 * for the <Image> component. `?w=400,800&image` picks the widths.
 *
 * Images are encoded with sharp, an optional dependency. Output names are
 * derived from the source's contents and the encoding settings, so the client
 * and server builds agree on URLs without sharing state, and encoded files
 * are cached in node_modules/.cache/tern-images between builds.
 */

/** Bumped when the encoding changes, so cached files are not reused. */
const VERSION = 1;
const DEFAULT_WIDTHS = [640, 1280, 1920];
const QUALITY = { avif: 50, webp: 75, jpeg: 80, png: 100 } as const;
const MIME = { avif: 'image/avif', webp: 'image/webp', jpeg: 'image/jpeg', png: 'image/png' } as const;
type Format = keyof typeof QUALITY;

const DEV_PREFIX = '@tern-image/';
const OUT_DIR = '_app/immutable/img/';

interface Variant {
	file: string;
	width: number;
	format: Format;
	key: string;
}

type Sharp = typeof import('sharp').default;

export function imagePlugin(): Plugin {
	let config: ResolvedConfig;
	let cacheDir: string;
	let sharpPromise: Promise<Sharp> | undefined;
	/** Dev: generated file name → how to make it. */
	const pending = new Map<string, Variant & { source: string }>();

	const sharp = () =>
		(sharpPromise ??= import('sharp').then(
			(m) => m.default,
			() => {
				throw new Error('tern: ?image imports need sharp. Install it with: npm install -D sharp');
			}
		));

	async function encode(source: string, v: Variant): Promise<Buffer> {
		const cached = path.join(cacheDir, `${v.key}.${v.format}`);
		try {
			return await fs.promises.readFile(cached);
		} catch {
			// not cached yet
		}
		const s = await sharp();
		const img = s(source).rotate().resize({ width: v.width, withoutEnlargement: true });
		const out = await img.toFormat(v.format, { quality: QUALITY[v.format] }).toBuffer();
		await fs.promises.mkdir(cacheDir, { recursive: true });
		await fs.promises.writeFile(cached, out);
		return out;
	}

	return {
		name: 'tern:image',
		enforce: 'pre',
		configResolved(c) {
			config = c;
			cacheDir = path.join(c.root, 'node_modules/.cache/tern-images');
		},
		async load(id) {
			const q = id.indexOf('?');
			if (q < 0) return undefined;
			const params = new URLSearchParams(id.slice(q + 1));
			if (!params.has('image')) return undefined;
			const source = id.slice(0, q);
			this.addWatchFile(source);

			const bytes = await fs.promises.readFile(source);
			const meta = await metadata(bytes, cacheDir, sharp);
			const fallback = fallbackFormat(meta.format, meta.hasAlpha);
			const widths = pickWidths(meta.width, params.get('w'));
			const formats: Format[] = fallback === 'webp' ? ['avif', 'webp'] : ['avif', 'webp', fallback];
			const contentHash = hash(bytes);
			const name = path.basename(source, path.extname(source)).replace(/[^\w-]/g, '_');

			const variants = new Map<Format, Variant[]>();
			for (const format of formats) {
				variants.set(
					format,
					widths.map((width) => {
						const key = hash(`${VERSION}:${contentHash}:${width}:${format}:${QUALITY[format]}`).slice(0, 10);
						return { file: `${name}-${width}-${key}.${format}`, width, format, key };
					})
				);
			}

			const isBuild = config.command === 'build';
			const isClient = !(this.environment?.config.consumer === 'server' || config.build.ssr);
			const url = (v: Variant) => (isBuild ? config.base + OUT_DIR + v.file : config.base + DEV_PREFIX + v.file);

			const all = [...variants.values()].flat();
			if (isBuild && isClient) {
				await Promise.all(
					all.map(async (v) => {
						this.emitFile({ type: 'asset', fileName: OUT_DIR + v.file, source: await encode(source, v) });
					})
				);
			} else if (!isBuild) {
				for (const v of all) pending.set(v.file, { ...v, source });
			}

			const srcset = (f: Format) =>
				variants
					.get(f)!
					.map((v) => `${url(v)} ${v.width}w`)
					.join(', ');
			const largest = variants.get(fallback)!.at(-1)!;
			const height = Math.round((meta.height * largest.width) / meta.width);
			const picture: Picture = {
				src: url(largest),
				width: largest.width,
				height,
				srcset: srcset(fallback),
				sources: formats.filter((f) => f !== fallback).map((f) => ({ type: MIME[f], srcset: srcset(f) }))
			};
			return `export default ${JSON.stringify(picture)};`;
		},
		configureServer(server) {
			server.middlewares.use((req, res, next) => {
				const at = req.url?.indexOf('/' + DEV_PREFIX) ?? -1;
				if (at < 0) return next();
				const file = decodeURIComponent(req.url!.slice(at + DEV_PREFIX.length + 1).split('?')[0]!);
				const v = pending.get(file);
				if (!v) return next();
				encode(v.source, v).then(
					(buf) => {
						res.setHeader('Content-Type', MIME[v.format]);
						res.setHeader('Cache-Control', 'no-cache');
						res.end(buf);
					},
					(err: unknown) => next(err)
				);
			});
		}
	};
}

function hash(data: string | Buffer): string {
	return createHash('sha256').update(data).digest('hex');
}

/** The widths to generate: the requested ones, or common widths below the image's, plus its own. */
export function pickWidths(original: number, requested: string | null): number[] {
	let widths: number[];
	if (requested) {
		widths = requested
			.split(/[,;]/)
			.map((w) => Number.parseInt(w, 10))
			.filter((w) => w > 0)
			.map((w) => Math.min(w, original));
		if (widths.length === 0) throw new Error(`tern: invalid image widths "${requested}"`);
	} else {
		widths = [...DEFAULT_WIDTHS.filter((w) => w < original), original];
	}
	return [...new Set(widths)].sort((a, b) => a - b);
}

function fallbackFormat(format: string | undefined, hasAlpha: boolean): Format {
	switch (format) {
		case 'jpeg':
		case 'jpg':
			return 'jpeg';
		case 'webp':
			return 'webp';
		case 'png':
		case 'gif':
			return 'png';
		default:
			return hasAlpha ? 'png' : 'jpeg';
	}
}

interface Meta {
	width: number;
	height: number;
	format?: string;
	hasAlpha: boolean;
}

async function metadata(bytes: Buffer, cacheDir: string, sharp: () => Promise<Sharp>): Promise<Meta> {
	const cached = path.join(cacheDir, `${hash(bytes).slice(0, 16)}.json`);
	try {
		return JSON.parse(await fs.promises.readFile(cached, 'utf8')) as Meta;
	} catch {
		// not cached yet
	}
	const s = await sharp();
	const m = await s(bytes).metadata();
	if (m.format === 'svg') throw new Error('tern: ?image is for raster images; import SVGs without ?image');
	if (!m.width || !m.height) throw new Error('tern: could not read the image size');
	// EXIF orientations 5–8 rotate by 90°, swapping width and height.
	const rotated = (m.orientation ?? 1) >= 5;
	const meta: Meta = {
		width: rotated ? m.height : m.width,
		height: rotated ? m.width : m.height,
		format: m.format,
		hasAlpha: m.hasAlpha ?? false
	};
	await fs.promises.mkdir(cacheDir, { recursive: true });
	await fs.promises.writeFile(cached, JSON.stringify(meta));
	return meta;
}
