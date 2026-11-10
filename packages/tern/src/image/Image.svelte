<script lang="ts">
	import type { HTMLImgAttributes } from 'svelte/elements';
	import type { Picture } from './types.js';

	interface Props extends Omit<HTMLImgAttributes, 'src' | 'srcset' | 'width' | 'height'> {
		/** The image, from `import picture from './photo.jpg?image'`. */
		src: Picture;
		/** Describes the image; use "" for decorative images. */
		alt: string;
		/**
		 * How wide the image is displayed, so the browser picks the right file,
		 * such as "(min-width: 768px) 50vw, 100vw". Defaults to its full width.
		 */
		sizes?: string;
		/** Load at once and with high priority, for the main image of a page. */
		priority?: boolean;
	}

	let { src, alt, sizes, priority = false, ...rest }: Props = $props();
	const fallbackSizes = $derived(`(min-width: ${src.width}px) ${src.width}px, 100vw`);
</script>

<picture>
	{#each src.sources as source (source.type)}
		<source type={source.type} srcset={source.srcset} sizes={sizes ?? fallbackSizes} />
	{/each}
	<img
		src={src.src}
		srcset={src.srcset}
		sizes={sizes ?? fallbackSizes}
		width={src.width}
		height={src.height}
		{alt}
		loading={priority ? 'eager' : 'lazy'}
		fetchpriority={priority ? 'high' : undefined}
		decoding="async"
		{...rest}
	/>
</picture>
