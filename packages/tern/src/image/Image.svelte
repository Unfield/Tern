<script lang="ts">
	import type { HTMLImgAttributes } from 'svelte/elements';
	import { getBase } from '../runtime/base.js';
	import type { Picture } from './types.js';

	interface Props extends Omit<HTMLImgAttributes, 'src' | 'srcset' | 'width' | 'height'> {
		/**
		 * The image: from `import picture from './photo.jpg?image'`, or the URL
		 * of a remote image, which the images plugin resizes (give its width
		 * and height then).
		 */
		src: Picture | string;
		/** Describes the image; use "" for decorative images. */
		alt: string;
		/** The width the image is shown at, in CSS pixels; required for URLs. */
		width?: number;
		/** The height for that width; required for URLs. */
		height?: number;
		/**
		 * How wide the image is displayed, so the browser picks the right file,
		 * such as "(min-width: 768px) 50vw, 100vw". Defaults to its full width.
		 */
		sizes?: string;
		/** Load at once and with high priority, for the main image of a page. */
		priority?: boolean;
	}

	let { src, alt, width, height, sizes, priority = false, ...rest }: Props = $props();

	// The widths the images plugin serves (plugins/images.Widths).
	const widths = [64, 128, 256, 384, 640, 750, 828, 1080, 1200, 1920, 2048, 3840];

	const picture = $derived.by((): Picture => {
		if (typeof src !== 'string') return src;
		const w = width ?? 1200;
		const h = height ?? Math.round(w * 0.75);
		// Up to twice the shown width, for high-density screens.
		const chosen = widths.filter((x) => x <= w * 2);
		if (!chosen.length || chosen.at(-1)! < w * 2) chosen.push(widths.find((x) => x >= w * 2) ?? widths.at(-1)!);
		const url = (x: number) => `${getBase()}/_tern/image?url=${encodeURIComponent(src as string)}&w=${x}`;
		return { src: url(chosen.find((x) => x >= w) ?? chosen.at(-1)!), width: w, height: h, srcset: chosen.map((x) => `${url(x)} ${x}w`).join(', '), sources: [] };
	});
	const fallbackSizes = $derived(`(min-width: ${picture.width}px) ${picture.width}px, 100vw`);
</script>

<picture>
	{#each picture.sources as source (source.type)}
		<source type={source.type} srcset={source.srcset} sizes={sizes ?? fallbackSizes} />
	{/each}
	<img
		src={picture.src}
		srcset={picture.srcset}
		sizes={sizes ?? fallbackSizes}
		width={picture.width}
		height={picture.height}
		{alt}
		loading={priority ? 'eager' : 'lazy'}
		fetchpriority={priority ? 'high' : undefined}
		decoding="async"
		{...rest}
	/>
</picture>
