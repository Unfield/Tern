/**
 * A clock for relative times: it starts at the server's time, so hydration
 * matches, and ticks in the browser.
 */
export function clock(serverNow: number, everyMs = 5000) {
	const state = $state({ now: serverNow });
	$effect(() => {
		state.now = Date.now();
		const t = setInterval(() => (state.now = Date.now()), everyMs);
		return () => clearInterval(t);
	});
	return state;
}
