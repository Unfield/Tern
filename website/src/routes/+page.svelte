<script lang="ts">
	import BenchmarkChart from '$lib/BenchmarkChart.svelte';
	import CodeWindow from '$lib/CodeWindow.svelte';
	import Command from '$lib/Command.svelte';
	import Icon from '$lib/Icon.svelte';
	import Terminal from '$lib/Terminal.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const table = $derived(data.benchmarks.find((b) => b.name.includes('100-row')));
	const nativeVsNode = $derived(table ? table.values[0] / table.values[table.values.length - 1] : 0);
	const fmt = new Intl.NumberFormat('en-US');

	const stats = $derived([
		{ value: table ? fmt.format(table.values[0]) : '', unit: 'req/s', label: 'a 100-row table rendered for every request, on one machine' },
		{ value: nativeVsNode ? nativeVsNode.toFixed(1) + '×' : '', unit: '', label: 'the throughput of SvelteKit on ten Node processes, for the same page' },
		{ value: '1', unit: 'binary', label: 'server, client, renderer and prerendered pages' },
		{ value: '0', unit: 'Node', label: 'processes in production' }
	]);

	const tree = [
		{ depth: 0, name: 'src/routes/', note: '' },
		{ depth: 1, name: '+layout.svelte', note: 'the shell of every page' },
		{ depth: 1, name: 'layout.server.go', note: 'its data, in Go' },
		{ depth: 1, name: 'blog/', note: '' },
		{ depth: 2, name: '+page.svelte', note: '/blog' },
		{ depth: 2, name: 'slug.param/', note: '' },
		{ depth: 3, name: '+page.svelte', note: '/blog/hello-world' },
		{ depth: 3, name: 'page.server.go', note: 'Load and Actions' },
		{ depth: 1, name: 'api/orders/', note: '' },
		{ depth: 2, name: 'server.go', note: 'GET, POST: a JSON API' }
	];

	const features = [
		{ title: 'Accounts, done', body: 'Sign-up and sign-in with passwords or passkeys, two-step codes, email verification, resets, devices, and GitHub or Google. A screen for each in Tern UI.', href: '/docs/accounts' },
		{ title: 'Forms checked twice', body: 'Struct tags validate a form in the browser as it’s typed and on the server when it’s sent, with the same messages in the reader’s language.', href: '/docs/guide#validating-forms' },
		{ title: 'Live components', body: 'Write a component’s logic in Go and run it on the server, with the markup typed and checked. State crosses one WebSocket.', href: '/docs/live-components' },
		{ title: 'Realtime', body: 'Server-sent events, WebSockets and publish-subscribe across processes, through PostgreSQL or in memory.', href: '/docs/guide#realtime' },
		{ title: 'Jobs, migrations, email, storage', body: 'Background jobs with retries and cron, SQL migrations, email over SMTP with previews in development, and files on disk or S3.', href: '/docs/guide#databases' },
		{ title: 'Logging', body: 'slog with request IDs, redaction and sampling, shipped to OpenTelemetry, Loki or Datadog, and a live log view in tern dev.', href: '/docs/logging' },
		{ title: 'Security signals', body: 'TLS, HTTP/2 and header fingerprints, a proof-of-work check and device IDs tell people from scripts, without tracking anyone.', href: '/docs/signals' },
		{ title: 'Tern UI', body: 'Accessible Svelte components: forms, dialogs, data tables, charts and the screens of sign-in, all yours to change.', href: '/docs/ui' },
		{ title: 'Translations and images', body: 'Messages per locale with plurals, localized routes, and responsive images resized and cached by the server.', href: '/docs/guide#translations' }
	];

	const deployLines = [
		{ kind: 'cmd' as const, text: 'go tool tern deploy production' },
		{ kind: 'blank' as const, text: '' },
		{ kind: 'dim' as const, text: '  tern deploy production  ssh · v1.4.0' },
		{ kind: 'blank' as const, text: '' },
		{ kind: 'ok' as const, text: 'Connected to root@203.0.113.5', note: 'linux/arm64' },
		{ kind: 'ok' as const, text: 'Built build/deploy/shop-linux-arm64', note: 'in 21.4s' },
		{ kind: 'ok' as const, text: 'Uploaded release 20261111-150405-v1.4.0', note: '14.2 MB in 2.1s' },
		{ kind: 'ok' as const, text: 'Starting release 20261111-150405-v1.4.0', note: 'ready; the old release stopped after its requests' },
		{ kind: 'link' as const, text: '    https://shop.example.com/' },
		{ kind: 'blank' as const, text: '' },
		{ kind: 'ok' as const, text: 'Deployed shop', note: 'in 26.8s' }
	];

	const providers = [
		{ name: 'A server over SSH', body: 'Any Linux machine with systemd. HTTPS from Let’s Encrypt, served by the app itself. Releases, rollbacks and logs.' },
		{ name: 'Fly.io', body: 'Writes fly.toml and a Dockerfile if the project has none, stages your secrets and deploys.' },
		{ name: 'Containers', body: 'Builds and pushes an image, then runs your platform’s deploy: Cloud Run, ECS, Kubernetes, Railway.' },
		{ name: 'Your own', body: 'Any provider is a program that gets the deployment as JSON, named tern-deploy-<name>.' }
	];
</script>

<svelte:head>
	<title>Tern: Svelte on the client, Go on the server</title>
	<meta name="description" content="Tern is a full-stack web framework with Svelte 5 on the client and Go on the server: file-based routes, typed loads and actions, server rendering compiled to Go, and one binary to deploy." />
	<meta property="og:title" content="Tern: Svelte on the client, Go on the server" />
	<meta property="og:description" content="File-based routes, typed data from Go to TypeScript, server rendering compiled to Go, and one binary to deploy." />
</svelte:head>

<!-- Hero -->
<section class="hero">
	<div class="backdrop" aria-hidden="true"></div>
	<div class="container hero-inner">
		<a class="announce" href="/docs/changelog">
			<span class="pill">New in 1.15</span>
			<span class="long">Admin screens, search, feature flags and dev tools</span>
			<span class="short">Admin, search and dev tools</span>
			<Icon name="arrow-right" size={14} />
		</a>
		<h1>Svelte on the client.<br /><span class="grad">Go on the server.</span></h1>
		<p class="lead">
			Tern is a full-stack framework with SvelteKit’s way of building apps and a Go backend: file-based routes, data typed from Go to TypeScript, server rendering compiled to Go, and one binary to deploy.
		</p>
		<div class="actions">
			<a class="btn btn-primary" href="/docs">Get started <Icon name="arrow-right" /></a>
			<a class="btn" href="/docs/tutorial">Follow the tutorial</a>
		</div>
		<div class="install">
			<Command command="npm create tern@latest" />
		</div>
	</div>
	<div class="container hero-code">
		<CodeWindow samples={data.hero} label="A Tern route: a Go load and action, and the Svelte page that shows its data" />
	</div>
</section>

<!-- Proof -->
<section class="stats" aria-label="Tern in numbers">
	<div class="container stats-grid">
		{#each stats as s (s.label)}
			<div class="stat">
				<p class="stat-value">{s.value}{#if s.unit}<span class="stat-unit">{s.unit}</span>{/if}</p>
				<p class="stat-label">{s.label}</p>
			</div>
		{/each}
	</div>
	<p class="container stats-note">Measured with the same Svelte components in both frameworks, <a href="/docs/benchmarks">on an Apple M4</a>.</p>
</section>

<!-- How it fits -->
<section class="section">
	<div class="container">
		<header class="section-head">
			<p class="eyebrow">How it fits together</p>
			<h2>Your routes are Svelte files and Go packages</h2>
			<p class="sub">Pages are real Svelte 5 components. The data next to them is plain Go: a Load function, form actions and endpoints, compiled into your server with everything else.</p>
		</header>

		<div class="bento">
			<article class="card span-7">
				<div class="card-text">
					<h3>Folders are routes</h3>
					<p>A page is a <code>+page.svelte</code>. Its server side is a Go file in the same folder, its own package. Parameters are folder names Go accepts.</p>
				</div>
				<div class="tree" role="img" aria-label="A routes folder: a layout, a blog index, a blog post with its Go load, and a JSON API">
					{#each tree as t, i (i)}
						<div class="tree-row" style:padding-left="{t.depth * 20}px">
							<span class="tree-name" class:dir={t.name.endsWith('/')} class:is-go={t.name.endsWith('.go')}>{t.name}</span>
							{#if t.note}<span class="tree-note">{t.note}</span>{/if}
						</div>
					{/each}
				</div>
			</article>

			<article class="card span-5">
				<div class="card-text">
					<h3>Typed from Go to TypeScript</h3>
					<p>Tern generates the types of every load, action and endpoint from your Go structs. Change a field in Go and <code>tern check</code> finds every page that reads it.</p>
				</div>
				<div class="mini-code">
					<CodeWindow samples={data.types} label="A Go struct and the TypeScript type Tern generates from it" />
				</div>
			</article>

			<article class="card span-5 highlight">
				<div class="card-text">
					<h3>Rendered by Go, not by Node</h3>
					<p><code>tern build</code> compiles Svelte’s server output to Go, so pages render in your process with no JavaScript runtime. What the compiler can’t handle falls back to an embedded engine, and the page still renders.</p>
				</div>
				<ul class="checks">
					<li><Icon name="check" /> Server-rendered HTML for every page, cached by its data</li>
					<li><Icon name="check" /> Streaming, prerendering, and selective reloads of layouts</li>
					<li><Icon name="check" /> Client navigation with typed links and preloading</li>
					<li><Icon name="check" /> One binary: <code>./build/app</code> is the whole app</li>
				</ul>
			</article>
			<article class="card span-7">
				<div class="card-text">
					<h3>Validated where it’s typed and where it lands</h3>
					<p>The same tags check the form in the browser and in Go. Errors arrive in the form, translated.</p>
				</div>
				<div class="mini-code">
					<CodeWindow samples={[data.validation]} label="Form validation rules as Go struct tags" />
				</div>
			</article>

		</div>
	</div>
</section>

<!-- Performance -->
<section class="section tinted">
	<div class="container">
		<header class="section-head">
			<p class="eyebrow">Performance</p>
			<h2>Faster than SvelteKit on ten Node processes</h2>
			<p class="sub">The same Svelte components, served by Tern and by SvelteKit on Node: navigating on the client, and rendering on the server for every request. Tern does it in one process, in a twelfth of the memory.</p>
		</header>
		<BenchmarkChart scenarios={data.benchmarks} />
		<p class="more"><a href="/docs/benchmarks">How these were measured <Icon name="arrow-right" size={14} /></a></p>
	</div>
</section>

<!-- Batteries -->
<section class="section">
	<div class="container">
		<header class="section-head">
			<p class="eyebrow">Included</p>
			<h2>The parts every app needs, built in</h2>
			<p class="sub">Each is a Go package or a plugin you turn on, documented and tested with the rest. Leave out what you don’t use: it isn’t in your binary.</p>
		</header>
		<div class="features">
			{#each features as f (f.title)}
				<a class="feature" href={f.href}>
					<h3>{f.title}</h3>
					<p>{f.body}</p>
					<span class="read-more">Read more <Icon name="arrow-right" size={14} /></span>
				</a>
			{/each}
		</div>
	</div>
</section>

<!-- Deploy -->
<section class="section tinted">
	<div class="container deploy">
		<div class="deploy-text">
			<p class="eyebrow">Shipping</p>
			<h2>One command to production</h2>
			<p class="sub">
				<code>tern deploy</code> builds for your server, uploads the release and starts it next to the running one. The old release stops only once the new one is ready, so no request is dropped. With a domain, the app serves HTTPS itself.
			</p>
			<div class="providers">
				{#each providers as p (p.name)}
					<div class="provider">
						<h3>{p.name}</h3>
						<p>{p.body}</p>
					</div>
				{/each}
			</div>
			<p class="more"><a href="/docs/deployment#tern-deploy">Deploying a Tern app <Icon name="arrow-right" size={14} /></a></p>
		</div>
		<Terminal lines={deployLines} title="tern deploy" />
	</div>
</section>

<!-- Start -->
<section class="section">
	<div class="container">
		<header class="section-head center">
			<p class="eyebrow">Start</p>
			<h2>A bare app, or a whole one</h2>
			<p class="sub">Choose when you create the project. Both are ordinary Tern apps: everything in them is yours to change.</p>
		</header>
		<div class="starts">
			<article class="start">
				<h3>Bare</h3>
				<p>A minimal app, a blog or a JSON API, with or without Tern UI. The fewest files to begin with.</p>
				<code class="start-cmd">go tool tern new -template minimal app</code>
			</article>
			<article class="start featured">
				<h3>Full</h3>
				<p>A SaaS starter with accounts, two-step sign-in, passkeys, a dashboard, settings, per-user data in SQLite and email in background jobs.</p>
				<code class="start-cmd">go tool tern new -template saas app</code>
			</article>
		</div>
	</div>
</section>

<!-- CTA -->
<section class="cta">
	<div class="container cta-inner">
		<h2>Build your next app with Tern</h2>
		<p>Svelte where your users are. Go where your data is.</p>
		<div class="actions">
			<a class="btn btn-primary" href="/docs">Get started <Icon name="arrow-right" /></a>
			<a class="btn" href="/examples">See the examples</a>
		</div>
	</div>
</section>

<style>
	/* Hero */
	.hero {
		position: relative;
		padding: var(--s-24) 0 var(--s-20);
		overflow: hidden;
	}
	.backdrop {
		position: absolute;
		inset: 0;
		z-index: -1;
		background-image:
			radial-gradient(ellipse 60% 50% at 50% 0%, var(--glow), transparent 70%),
			linear-gradient(var(--grid-line) 1px, transparent 1px),
			linear-gradient(90deg, var(--grid-line) 1px, transparent 1px);
		background-size:
			100% 100%,
			56px 56px,
			56px 56px;
		background-position: center top;
		mask-image: linear-gradient(to bottom, black 40%, transparent 92%);
	}
	.hero-inner {
		display: flex;
		flex-direction: column;
		align-items: center;
		text-align: center;
	}
	.announce {
		display: inline-flex;
		align-items: center;
		gap: var(--s-3);
		height: 34px;
		padding: 0 var(--s-4) 0 var(--s-1);
		border: 1px solid var(--border);
		border-radius: 999px;
		background: var(--surface);
		color: var(--text-2);
		font-size: 13.5px;
		box-shadow: var(--shadow);
		transition: border-color 160ms, color 160ms;
	}
	.announce:hover {
		color: var(--text);
		border-color: var(--border-strong);
	}
	.announce :global(code) {
		font-size: 12.5px;
	}
	.announce .short {
		display: none;
	}
	.pill {
		padding: 3px var(--s-3);
		border-radius: 999px;
		background: var(--accent-soft);
		color: var(--accent);
		font-size: 12px;
		font-weight: 600;
	}
	h1 {
		margin-top: var(--s-8);
		font-size: clamp(42px, 7.2vw, 80px);
		font-weight: 720;
		line-height: 1.02;
		letter-spacing: -0.045em;
	}
	.grad {
		background: linear-gradient(100deg, var(--text) 10%, var(--brand) 55%, var(--accent) 95%);
		-webkit-background-clip: text;
		background-clip: text;
		color: transparent;
	}
	.lead {
		max-width: 680px;
		margin-top: var(--s-6);
		color: var(--text-2);
		font-size: clamp(17px, 2vw, 20px);
		line-height: 1.55;
	}
	.actions {
		display: flex;
		flex-wrap: wrap;
		justify-content: center;
		gap: var(--s-3);
		margin-top: var(--s-10);
	}
	.install {
		margin-top: var(--s-6);
	}
	.hero-code {
		max-width: calc(940px + 2 * var(--gutter));
		margin-top: var(--s-16);
	}

	/* Stats */
	.stats {
		padding: var(--s-16) 0 var(--s-12);
		border-top: 1px solid var(--border);
		border-bottom: 1px solid var(--border);
		background: var(--bg-2);
	}
	.stats-grid {
		display: grid;
		grid-template-columns: repeat(4, 1fr);
		gap: var(--s-8);
	}
	.stat-value {
		color: var(--text);
		font-size: clamp(32px, 4vw, 44px);
		font-weight: 680;
		letter-spacing: -0.035em;
		line-height: 1.1;
		font-variant-numeric: tabular-nums;
	}
	.stat-unit {
		margin-left: var(--s-2);
		color: var(--text-3);
		font-size: 0.45em;
		font-weight: 550;
		letter-spacing: 0;
	}
	.stat-label {
		max-width: 240px;
		margin-top: var(--s-2);
		color: var(--text-2);
		font-size: 14.5px;
		line-height: 1.5;
	}
	.stats-note {
		margin-top: var(--s-10);
		color: var(--text-3);
		font-size: 13px;
	}

	/* Sections */
	.section {
		padding: var(--s-32) 0;
	}
	.tinted {
		border-top: 1px solid var(--border);
		border-bottom: 1px solid var(--border);
		background: var(--bg-2);
	}
	.section-head {
		max-width: 720px;
		margin-bottom: var(--s-16);
	}
	.section-head.center {
		margin-left: auto;
		margin-right: auto;
		text-align: center;
	}
	h2 {
		margin-top: var(--s-4);
		font-size: clamp(30px, 4.2vw, 46px);
		font-weight: 680;
		line-height: 1.08;
		letter-spacing: -0.035em;
	}
	.sub {
		margin-top: var(--s-5);
		color: var(--text-2);
		font-size: 18px;
		line-height: 1.6;
	}
	.more {
		margin-top: var(--s-8);
		font-size: 15px;
		font-weight: 550;
	}
	.more a {
		display: inline-flex;
		align-items: center;
		gap: var(--s-2);
	}
	code {
		font-size: 0.88em;
	}
	p code,
	li code {
		padding: 1px 6px;
		border: 1px solid var(--border);
		border-radius: 5px;
		background: var(--surface-2);
		color: var(--text);
	}

	/* Bento */
	.bento {
		display: grid;
		grid-template-columns: repeat(12, 1fr);
		gap: var(--s-6);
	}
	.span-7 {
		grid-column: span 7;
	}
	.span-5 {
		grid-column: span 5;
	}
	.card {
		display: flex;
		flex-direction: column;
		gap: var(--s-8);
		min-width: 0;
		padding: var(--s-10);
		border: 1px solid var(--border);
		border-radius: var(--radius-xl);
		background: var(--surface);
	}
	.card.highlight {
		background:
			radial-gradient(ellipse 80% 60% at 100% 0%, var(--glow), transparent 70%),
			var(--surface);
	}
	.card h3 {
		font-size: 22px;
		font-weight: 650;
		letter-spacing: -0.02em;
	}
	.card-text p {
		margin-top: var(--s-3);
		color: var(--text-2);
		font-size: 15.5px;
		line-height: 1.6;
	}
	.mini-code :global(.window) {
		box-shadow: none;
	}
	.tree {
		padding: var(--s-5) var(--s-6);
		border: 1px solid var(--border);
		border-radius: var(--radius-l);
		background: var(--code-bg);
		font-family: var(--mono);
		font-size: 13px;
		line-height: 2;
		overflow-x: auto;
	}
	.tree-row {
		display: flex;
		gap: var(--s-4);
		white-space: nowrap;
	}
	.tree-name {
		color: var(--text);
	}
	.tree-name.dir {
		color: var(--text-2);
		font-weight: 600;
	}
	.tree-name.is-go {
		color: var(--accent);
	}
	.tree-note {
		color: var(--text-3);
		font-family: var(--font);
		font-size: 12.5px;
	}
	.checks {
		display: flex;
		flex-direction: column;
		gap: var(--s-4);
		margin: 0;
		padding: 0;
		list-style: none;
	}
	.checks li {
		display: flex;
		align-items: flex-start;
		gap: var(--s-3);
		color: var(--text-2);
		font-size: 15px;
		line-height: 1.5;
	}
	.checks :global(svg) {
		flex: none;
		margin-top: 3px;
		color: var(--accent);
	}

	/* Features */
	.features {
		display: grid;
		grid-template-columns: repeat(3, 1fr);
		gap: var(--s-6);
	}
	.feature {
		display: flex;
		flex-direction: column;
		padding: var(--s-8);
		border: 1px solid var(--border);
		border-radius: var(--radius-l);
		background: var(--surface);
		color: var(--text);
		transition:
			border-color 180ms var(--ease),
			transform 180ms var(--ease),
			box-shadow 180ms var(--ease);
	}
	.feature:hover {
		color: var(--text);
		border-color: var(--accent-line);
		box-shadow: var(--shadow);
		transform: translateY(-2px);
	}
	.feature h3 {
		font-size: 17px;
		font-weight: 620;
		letter-spacing: -0.01em;
	}
	.feature p {
		flex: 1;
		margin-top: var(--s-3);
		color: var(--text-2);
		font-size: 14.5px;
		line-height: 1.6;
	}
	.read-more {
		display: inline-flex;
		align-items: center;
		gap: var(--s-2);
		margin-top: var(--s-6);
		color: var(--accent);
		font-size: 14px;
		font-weight: 550;
	}

	/* Deploy */
	.deploy {
		display: grid;
		grid-template-columns: 1fr 1.1fr;
		align-items: center;
		gap: var(--s-16);
	}
	.providers {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: var(--s-6) var(--s-8);
		margin-top: var(--s-10);
	}
	.provider h3 {
		font-size: 15px;
		font-weight: 620;
		letter-spacing: 0;
	}
	.provider p {
		margin-top: var(--s-2);
		color: var(--text-2);
		font-size: 14px;
		line-height: 1.55;
	}

	/* Start */
	.starts {
		display: grid;
		grid-template-columns: 1fr 1fr;
		gap: var(--s-6);
		max-width: 920px;
		margin: 0 auto;
	}
	.start {
		display: flex;
		flex-direction: column;
		padding: var(--s-10);
		border: 1px solid var(--border);
		border-radius: var(--radius-xl);
		background: var(--surface);
	}
	.start.featured {
		border-color: var(--accent-line);
		background:
			radial-gradient(ellipse 70% 60% at 100% 0%, var(--glow), transparent 70%),
			var(--surface);
	}
	.start h3 {
		font-size: 22px;
		font-weight: 650;
	}
	.start p {
		flex: 1;
		margin-top: var(--s-3);
		color: var(--text-2);
		font-size: 15.5px;
	}
	.start-cmd {
		display: block;
		margin-top: var(--s-8);
		padding: var(--s-3) var(--s-4);
		border: 1px solid var(--border);
		border-radius: var(--radius);
		background: var(--code-bg);
		color: var(--text);
		font-size: 13px;
		overflow-x: auto;
		white-space: nowrap;
	}

	/* CTA */
	.cta {
		padding: var(--s-32) 0;
		border-top: 1px solid var(--border);
		background:
			radial-gradient(ellipse 50% 70% at 50% 100%, var(--glow), transparent 70%),
			var(--bg);
	}
	.cta-inner {
		text-align: center;
	}
	.cta h2 {
		margin: 0;
	}
	.cta p {
		margin-top: var(--s-4);
		color: var(--text-2);
		font-size: 18px;
	}

	@media (max-width: 1024px) {
		.span-7,
		.span-5 {
			grid-column: span 12;
		}
		.features {
			grid-template-columns: repeat(2, 1fr);
		}
		.deploy {
			grid-template-columns: 1fr;
			gap: var(--s-12);
		}
	}
	@media (max-width: 760px) {
		.stats-grid {
			grid-template-columns: repeat(2, 1fr);
			gap: var(--s-10) var(--s-6);
		}
	}
	@media (max-width: 640px) {
		.announce .long {
			display: none;
		}
		.announce .short {
			display: inline;
		}
		.hero {
			padding: var(--s-16) 0 var(--s-12);
		}
		.hero-code {
			margin-top: var(--s-12);
		}
		.actions {
			margin-top: var(--s-8);
		}
		.section,
		.cta {
			padding: var(--s-20) 0;
		}
		.section-head {
			margin-bottom: var(--s-10);
		}
		.sub {
			font-size: 16.5px;
		}
		.card,
		.start {
			padding: var(--s-6);
		}
		.tree-note {
			display: none;
		}
		.feature {
			padding: var(--s-6);
		}
		.features,
		.starts,
		.providers {
			grid-template-columns: 1fr;
		}
	}
</style>
