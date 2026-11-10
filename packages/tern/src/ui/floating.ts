// Positions a floating element (a popover, menu or tooltip) next to an
// anchor element or a point, flipping to the other side and shifting along
// it to stay in the viewport. The floating element is position: fixed,
// usually in the top layer through the Popover API.

export type Side = 'top' | 'bottom' | 'left' | 'right';
export type Align = 'start' | 'center' | 'end';

export type FloatingOptions = {
	side?: Side;
	align?: Align;
	/** The gap between the anchor and the floating element, in pixels. */
	offset?: number;
	/** The space kept free at the viewport's edges, in pixels. */
	padding?: number;
	/** Make the floating element at least as wide as the anchor. */
	matchWidth?: boolean;
};

type Rect = { x: number; y: number; width: number; height: number };

const opposite: Record<Side, Side> = { top: 'bottom', bottom: 'top', left: 'right', right: 'left' };

/** Computes where the floating element goes. Exported for tests. */
export function compute(anchor: Rect, size: { width: number; height: number }, viewport: { width: number; height: number }, o: FloatingOptions = {}) {
	const { side = 'bottom', align = 'start', offset = 6, padding = 8 } = o;
	const place = (s: Side) => {
		const vertical = s === 'top' || s === 'bottom';
		let x: number;
		let y: number;
		if (vertical) {
			y = s === 'bottom' ? anchor.y + anchor.height + offset : anchor.y - size.height - offset;
			x = align === 'start' ? anchor.x : align === 'end' ? anchor.x + anchor.width - size.width : anchor.x + (anchor.width - size.width) / 2;
		} else {
			x = s === 'right' ? anchor.x + anchor.width + offset : anchor.x - size.width - offset;
			y = align === 'start' ? anchor.y : align === 'end' ? anchor.y + anchor.height - size.height : anchor.y + (anchor.height - size.height) / 2;
		}
		return { x, y };
	};
	const fits = (s: Side, p: { x: number; y: number }) =>
		s === 'bottom' ? p.y + size.height <= viewport.height - padding : s === 'top' ? p.y >= padding : s === 'right' ? p.x + size.width <= viewport.width - padding : p.x >= padding;

	let chosen = side;
	let p = place(side);
	if (!fits(side, p)) {
		const q = place(opposite[side]);
		if (fits(opposite[side], q)) {
			chosen = opposite[side];
			p = q;
		}
	}
	// Shift along the side to stay on screen.
	const clamp = (v: number, min: number, max: number) => Math.max(min, Math.min(v, Math.max(min, max)));
	p.x = clamp(p.x, padding, viewport.width - size.width - padding);
	p.y = clamp(p.y, padding, viewport.height - size.height - padding);
	return { x: Math.round(p.x), y: Math.round(p.y), side: chosen };
}

/**
 * Keeps el next to anchor while it is shown: on scroll, resize and size
 * changes. Returns a function that stops. el gets data-side with the side
 * it ended up on, for styles.
 */
export function float(el: HTMLElement, anchor: Element | { x: number; y: number }, o: FloatingOptions = {}): () => void {
	el.style.position = 'fixed';
	el.style.margin = '0';
	el.style.inset = 'auto';
	const update = () => {
		const r: Rect = anchor instanceof Element ? anchor.getBoundingClientRect() : { x: anchor.x, y: anchor.y, width: 0, height: 0 };
		if (o.matchWidth) el.style.minWidth = `${r.width}px`;
		const pos = compute(r, { width: el.offsetWidth, height: el.offsetHeight }, { width: document.documentElement.clientWidth, height: window.innerHeight }, o);
		el.style.left = `${pos.x}px`;
		el.style.top = `${pos.y}px`;
		el.dataset.side = pos.side;
	};
	update();
	let frame = 0;
	const schedule = () => {
		cancelAnimationFrame(frame);
		frame = requestAnimationFrame(update);
	};
	window.addEventListener('scroll', schedule, { capture: true, passive: true });
	window.addEventListener('resize', schedule);
	const ro = new ResizeObserver(schedule);
	ro.observe(el);
	if (anchor instanceof Element) ro.observe(anchor);
	return () => {
		cancelAnimationFrame(frame);
		window.removeEventListener('scroll', schedule, { capture: true });
		window.removeEventListener('resize', schedule);
		ro.disconnect();
	};
}

/** The elements inside root that can take focus, in order. */
export function focusables(root: Element): HTMLElement[] {
	return Array.from(
		root.querySelectorAll<HTMLElement>(
			'a[href], button:not([disabled]), input:not([disabled]):not([type="hidden"]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
		)
	).filter((el) => el.offsetParent !== null || el === document.activeElement);
}

/** Keeps Tab inside root, as modal dialogs need. Call it from keydown. */
export function trapTab(root: Element, e: KeyboardEvent) {
	if (e.key !== 'Tab') return;
	const list = focusables(root);
	if (list.length === 0) return;
	const first = list[0]!;
	const last = list[list.length - 1]!;
	if (e.shiftKey && document.activeElement === first) {
		e.preventDefault();
		last.focus();
	} else if (!e.shiftKey && document.activeElement === last) {
		e.preventDefault();
		first.focus();
	}
}
