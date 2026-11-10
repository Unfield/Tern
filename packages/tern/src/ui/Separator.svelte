<script lang="ts">
	type Props = { vertical?: boolean; class?: string };
	let { vertical = false, class: klass = '' }: Props = $props();
</script>

<div class="tern-separator {klass}" class:vertical role="separator" aria-orientation={vertical ? 'vertical' : 'horizontal'}></div>

<style>
	.tern-separator {
		height: 1px;
		width: 100%;
		background: var(--tern-border);
	}
	.vertical {
		width: 1px;
		height: auto;
		align-self: stretch;
	}
</style>
