// Tern analytics: counts page views without cookies. Add it to a site with
// <script defer data-domain="example.com" src="https://analytics.example/script.js"></script>
(function () {
	var s = document.currentScript;
	if (!s) return;
	var domain = s.getAttribute('data-domain') || location.hostname;
	var endpoint = new URL('/api/event', s.src).href;
	var last = '';
	function send() {
		if (location.href === last) return;
		last = location.href;
		if (/^localhost$|^127\./.test(location.hostname) && !s.hasAttribute('data-local')) return;
		var body = JSON.stringify({ d: domain, u: location.href, r: document.referrer });
		if (navigator.sendBeacon && navigator.sendBeacon(endpoint, body)) return;
		fetch(endpoint, { method: 'POST', body: body, keepalive: true, credentials: 'omit' });
	}
	// Single-page apps change pages without loading them.
	var push = history.pushState;
	history.pushState = function () {
		push.apply(this, arguments);
		send();
	};
	addEventListener('popstate', send);
	if (document.visibilityState === 'prerender') document.addEventListener('visibilitychange', send, { once: true });
	else send();
})();
