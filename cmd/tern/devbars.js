// Sticky and fixed bars off the pixel grid. While the page scrolls, the
// browser rounds what's inside such a bar to the screen's pixels, up in one
// frame and down in the next, so the bar and its text twitch by a pixel. The
// cause is in the app's CSS (sizes in rem, em or unitless line heights that
// don't come out whole), so tern dev measures each bar after the page
// settles and says which element is off, and where it's written.
(() => {
	if (window.__ternBars) return;
	window.__ternBars = true;
	const said = new Set();
	const off = (v) => Math.abs(v - Math.round(v)) > 0.02;
	const px = (v) => Math.round(v * 100) / 100 + 'px';

	function name(el) {
		const cls = [...el.classList].filter((c) => !c.startsWith('svelte-')).slice(0, 2);
		return el.localName + (el.id ? '#' + el.id : '') + cls.map((c) => '.' + c).join('');
	}
	function where(el) {
		// Svelte's dev build notes where each element is written.
		const loc = el.__svelte_meta?.loc;
		if (!loc?.file) return '';
		return ` (${String(loc.file).replace(/^.*?\/src\//, 'src/')}:${loc.line})`;
	}

	function check() {
		const vw = innerWidth;
		const vh = innerHeight;
		if (!vw || !vh) return; // a hidden tab: nothing is laid out
		const all = document.body ? document.body.getElementsByTagName('*') : [];
		if (all.length > 8000) return;
		for (const bar of all) {
			if (bar.closest('tern-dev-overlay, dialog, [popover], [role="dialog"]')) continue;
			const pos = getComputedStyle(bar).position;
			if (pos !== 'sticky' && pos !== 'fixed') continue;
			const box = bar.getBoundingClientRect();
			if (!box.height || !box.width) continue;
			// A bar spans an edge: a header, a sidebar. Toasts and the like don't count.
			if (box.width < vw / 2 && box.height < vh / 2) continue;
			let what = '';
			if (off(box.height) && box.height < vh / 2) what = `is ${px(box.height)} tall`;
			if (!what) {
				const inside = bar.getElementsByTagName('*');
				for (let i = 0; i < inside.length && i < 400; i++) {
					const el = inside[i];
					// The shapes in an icon sit where they're drawn; the icon itself counts.
					if (el.ownerSVGElement) continue;
					const r = el.getBoundingClientRect();
					if (!r.height) continue;
					const y = r.top - box.top;
					if (off(y) || off(r.height)) {
						what = `has content off it: ${name(el)}${where(el)} is ${off(r.height) ? `${px(r.height)} tall` : `${px(y)} from the bar's top`}`;
						break;
					}
				}
			}
			if (!what) continue;
			const msg = `The ${pos} ${box.height < vh / 2 ? 'bar' : 'sidebar'} ${name(bar)}${where(bar)} isn't on the pixel grid: it ${what}. Browsers round it differently from frame to frame, so it twitches while the page scrolls. Give the bar and what's inside whole-pixel sizes: px heights, paddings and line heights (see "Sticky and fixed bars" in docs/ui.md).`;
			if (said.has(msg)) continue;
			said.add(msg);
			console.warn('[tern] ' + msg);
			fetch('/__tern/dev/api/warn', { method: 'POST', credentials: 'same-origin', headers: { 'content-type': 'application/json' }, body: JSON.stringify({ message: msg, url: location.pathname }) }).catch(() => {});
		}
	}

	// After the page settles: on load, and after the DOM changes (client
	// navigation, data arriving), at most every two seconds.
	let timer = 0;
	const later = () => {
		clearTimeout(timer);
		timer = setTimeout(() => (window.requestIdleCallback ?? setTimeout)(check), 1000);
	};
	if (document.readyState === 'complete') later();
	else addEventListener('load', later);
	addEventListener('resize', later);
	let last = 0;
	new MutationObserver(() => {
		const now = Date.now();
		if (now - last < 2000) return;
		last = now;
		later();
	}).observe(document.documentElement, { childList: true, subtree: true });
})();
