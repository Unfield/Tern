// The color theme: the reader's choice, kept in localStorage, or the
// system's. app.html sets it before the first paint.
class Theme {
	current = $state<'light' | 'dark'>('light');

	init() {
		const t = document.documentElement.dataset.theme;
		this.current = t === 'dark' ? 'dark' : 'light';
	}

	toggle() {
		this.current = this.current === 'dark' ? 'light' : 'dark';
		document.documentElement.dataset.theme = this.current;
		try {
			localStorage.setItem('tern-theme', this.current);
		} catch {
			// private windows: the choice lasts for the page
		}
	}
}

export const theme = new Theme();
