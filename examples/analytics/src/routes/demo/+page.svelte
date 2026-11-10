<svelte:head>
	<title>Demo page</title>
	<!-- The tracking script, as a tracked site adds it. data-local counts views on localhost. -->
	<script defer data-domain="demo.example" data-local src="/script.js"></script>
</svelte:head>

<main style="padding: 2rem; font-family: system-ui">
	<h1>A page on demo.example</h1>
	<p>Opening this page counts a view for the demo site.</p>
	<a href="/demo/about">About</a>
</main>
