// The same items as the Tern app's load, from the same formula.
export function load() {
	const start = Date.UTC(2026, 0, 1);
	const items = Array.from({ length: 100 }, (_, i) => {
		const n = i + 1;
		return {
			id: n,
			name: `Item ${n}`,
			price: ((n * 37) % 1000) / 10 + 0.99,
			tags: [`tag${n % 5}`, `tag${(n % 7) + 5}`, `tag${(n % 3) + 12}`],
			updated: new Date(start + n * 86_400_000).toISOString().replace('.000Z', 'Z'),
			stock: (n * 13) % 9
		};
	});
	return { items };
}
