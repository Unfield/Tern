<script lang="ts">
	import { marked } from 'marked';
	import hljs from 'highlight.js/lib/core';
	import go from 'highlight.js/lib/languages/go';
	import katex from 'katex';
	hljs.registerLanguage('go', go);
	const html = marked.parse('# Title\n\nSome **bold** text.', { async: false }) as string;
	const code = hljs.highlight('func main() {}', { language: 'go' }).value;
	const math = katex.renderToString('E = mc^2', { output: 'html' });
</script>

{@html html}
<pre>{@html code}</pre>
<div class="math">{@html math}</div>
