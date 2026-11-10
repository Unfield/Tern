/** A responsive image from an `import x from './x.jpg?image'`. */
export interface Picture {
	/** The largest version in the image's own format, for browsers without srcset. */
	src: string;
	/** The largest version's size, so the browser reserves space before it loads. */
	width: number;
	height: number;
	/** Every width in the image's own format. */
	srcset: string;
	/** AVIF and WebP versions, best first. */
	sources: { type: string; srcset: string }[];
}
