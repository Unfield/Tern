package main

import (
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
)

func chromaStyle(name string) *chroma.Style {
	if s := styles.Get(name); s != nil {
		return s
	}
	return styles.Fallback
}

const pageTemplate = `<!doctype html>
<html lang="en">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>{{if .Page.Slug}}{{.Page.Title}} · {{end}}Tern: Svelte on the client, Go on the server</title>
	<meta name="description" content="Tern is a full-stack web framework with Svelte 5 on the client and Go on the server: file-based routing, typed data loading, server-side rendering and single-binary deploys.">
	<link rel="icon" href="{{.Base}}logo.svg">
	<link rel="stylesheet" href="{{.Base}}site.css">
</head>
<body>
	<header class="top">
		<a class="brand" href="{{.Base}}"><img src="{{.Base}}logo.svg" alt="" width="28" height="28"> Tern</a>
		<nav class="top-links">
			<a href="{{.Base}}guide/">Guide</a>
			<a href="{{.Repo}}">GitHub</a>
		</nav>
		<label class="menu-toggle" for="menu">Menu</label>
	</header>
	<input type="checkbox" id="menu" hidden>
	<div class="layout">
		<aside class="sidebar">
			{{range .Nav}}
			<p class="section">{{.Name}}</p>
			<ul>
				{{range .Links}}<li><a href="{{.URL}}"{{if .Active}} aria-current="page"{{end}}>{{.Title}}</a></li>{{end}}
			</ul>
			{{end}}
		</aside>
		<main class="content">
			{{if not .Page.Slug}}
			<section class="hero">
				<img src="{{.Base}}logo.svg" alt="" width="72" height="72">
				<h1>Svelte on the client.<br>Go on the server.</h1>
				<p>Tern is a full-stack framework with SvelteKit's developer experience and a Go backend: file-based routes, typed data loading, server-side rendering, and one binary to deploy.</p>
				<div class="cta">
					<a class="button primary" href="{{.Base}}guide/">Read the guide</a>
					<a class="button" href="{{.Repo}}">View on GitHub</a>
				</div>
			</section>
			{{end}}
			<article>{{.Page.HTML}}</article>
			<footer class="page-footer">
				<a href="{{.Edit}}">Edit this page on GitHub</a>
				<span>MIT licensed</span>
			</footer>
		</main>
	</div>
</body>
</html>
`

const siteCSS = `:root {
	--bg: #ffffff;
	--fg: #1c1917;
	--muted: #57534e;
	--border: #e7e5e4;
	--soft: #f5f5f4;
	--accent: #0f766e;
	--code-bg: #f6f8fa;
	color-scheme: light dark;
}
@media (prefers-color-scheme: dark) {
	:root {
		--bg: #0c0a09;
		--fg: #f5f5f4;
		--muted: #a8a29e;
		--border: #292524;
		--soft: #1c1917;
		--accent: #2dd4bf;
		--code-bg: #161b22;
	}
}
* { box-sizing: border-box; }
html { scroll-padding-top: 4.5rem; }
body { margin: 0; font: 16px/1.65 ui-sans-serif, system-ui, -apple-system, "Segoe UI", sans-serif; color: var(--fg); background: var(--bg); }
a { color: var(--accent); text-underline-offset: 2px; }
.top { position: sticky; top: 0; z-index: 10; display: flex; align-items: center; gap: 1.5rem; height: 3.5rem; padding: 0 1.25rem; background: color-mix(in srgb, var(--bg) 85%, transparent); backdrop-filter: blur(8px); border-bottom: 1px solid var(--border); }
.brand { display: flex; align-items: center; gap: 0.5rem; font-weight: 700; font-size: 1.1rem; color: var(--fg); text-decoration: none; }
.top-links { display: flex; gap: 1.25rem; margin-left: auto; }
.top-links a { color: var(--muted); text-decoration: none; font-size: 0.95rem; }
.top-links a:hover { color: var(--fg); }
.menu-toggle { display: none; cursor: pointer; color: var(--muted); }
.layout { display: grid; grid-template-columns: 16rem minmax(0, 1fr); max-width: 80rem; margin: 0 auto; }
.sidebar { position: sticky; top: 3.5rem; height: calc(100vh - 3.5rem); overflow-y: auto; padding: 1.5rem 1rem 2rem 1.25rem; border-right: 1px solid var(--border); font-size: 0.93rem; }
.sidebar .section { margin: 1.25rem 0 0.35rem; font-size: 0.75rem; font-weight: 600; letter-spacing: 0.04em; text-transform: uppercase; color: var(--muted); }
.sidebar .section:first-child { margin-top: 0; }
.sidebar ul { list-style: none; margin: 0; padding: 0; }
.sidebar a { display: block; padding: 0.25rem 0.5rem; border-radius: 6px; color: var(--fg); text-decoration: none; }
.sidebar a:hover { background: var(--soft); }
.sidebar a[aria-current="page"] { background: var(--soft); color: var(--accent); font-weight: 600; }
.content { padding: 2rem 2.5rem 4rem; min-width: 0; }
article { max-width: 48rem; }
article h1 { font-size: 2.25rem; line-height: 1.2; letter-spacing: -0.02em; margin: 0 0 1rem; }
article h2 { font-size: 1.5rem; margin: 2.5rem 0 0.75rem; padding-top: 0.5rem; border-top: 1px solid var(--border); }
article h3 { font-size: 1.15rem; margin: 1.75rem 0 0.5rem; }
article code { font: 0.875em ui-monospace, SFMono-Regular, Menlo, monospace; background: var(--code-bg); padding: 0.15em 0.35em; border-radius: 4px; }
article pre { background: var(--code-bg); padding: 1rem 1.1rem; border-radius: 8px; overflow-x: auto; line-height: 1.5; border: 1px solid var(--border); }
article pre code { background: none; padding: 0; font-size: 0.85rem; }
article table { border-collapse: collapse; width: 100%; margin: 1rem 0; font-size: 0.93rem; display: block; overflow-x: auto; }
article th, article td { border: 1px solid var(--border); padding: 0.45rem 0.7rem; text-align: left; vertical-align: top; }
article th { background: var(--soft); }
article blockquote { margin: 1rem 0; padding: 0.5rem 1rem; border-left: 3px solid var(--accent); background: var(--soft); border-radius: 0 6px 6px 0; }
article img { max-width: 100%; }
.hero { max-width: 48rem; padding: 1.5rem 0 1rem; }
.hero h1 { font-size: 2.75rem; line-height: 1.1; letter-spacing: -0.03em; margin: 1rem 0; }
.hero p { font-size: 1.15rem; color: var(--muted); margin: 0 0 1.5rem; }
.cta { display: flex; flex-wrap: wrap; gap: 0.75rem; }
.button { display: inline-block; padding: 0.6rem 1.1rem; border-radius: 8px; border: 1px solid var(--border); color: var(--fg); text-decoration: none; font-weight: 600; }
.button.primary { background: var(--accent); border-color: var(--accent); color: var(--bg); }
.page-footer { display: flex; justify-content: space-between; max-width: 48rem; margin-top: 4rem; padding-top: 1rem; border-top: 1px solid var(--border); font-size: 0.9rem; color: var(--muted); }
@media (max-width: 52rem) {
	.layout { grid-template-columns: 1fr; }
	.sidebar { display: none; position: static; height: auto; border-right: none; border-bottom: 1px solid var(--border); }
	#menu:checked + .layout .sidebar { display: block; }
	.menu-toggle { display: block; }
	.content { padding: 1.5rem 1.1rem 3rem; }
	article h1 { font-size: 1.8rem; }
}
`
