<script lang="ts">
	import { _, addMessages, init } from 'svelte-i18n';
	addMessages('en', { hello: 'Hello {name}', items: '{n, plural, one {# item} other {# items}}' });
	init({ fallbackLocale: 'en', initialLocale: 'en' });
</script>

<p>{$_('hello', { values: { name: 'Tern' } })} / {$_('items', { values: { n: 3 } })}</p>
