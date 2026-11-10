<script lang="ts">
	import { navigating, page } from '@ternkit/tern';
	import { toggleMode } from 'mode-watcher';
	import { toast } from 'svelte-sonner';
	import Bell from '@lucide/svelte/icons/bell';
	import ExternalLink from '@lucide/svelte/icons/external-link';
	import Gauge from '@lucide/svelte/icons/gauge';
	import LogOut from '@lucide/svelte/icons/log-out';
	import Megaphone from '@lucide/svelte/icons/megaphone';
	import Moon from '@lucide/svelte/icons/moon';
	import Settings from '@lucide/svelte/icons/settings';
	import Logo from '$lib/Logo.svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import type { LayoutProps } from './$types';

	let { data, children }: LayoutProps = $props();

	const nav = [
		{ href: '/admin', label: 'Monitors', icon: Gauge },
		{ href: '/admin/incidents', label: 'Incidents', icon: Megaphone },
		{ href: '/admin/notifications', label: 'Notifications', icon: Bell },
		{ href: '/admin/settings', label: 'Settings', icon: Settings }
	];
	const active = (href: string) => (href === '/admin' ? page.url.pathname === '/admin' || page.url.pathname.startsWith('/admin/monitors') : page.url.pathname.startsWith(href));

	$effect(() => {
		if (data.notice) toast.success(data.notice);
	});
</script>

{#if navigating.current}<div class="bg-primary fixed inset-x-0 top-0 z-50 h-0.5 animate-pulse" aria-hidden="true"></div>{/if}

<div class="flex min-h-svh flex-col">
	<header class="bg-background/80 sticky top-0 z-40 border-b backdrop-blur">
		<div class="mx-auto flex h-14 max-w-6xl items-center gap-6 px-4">
			<Logo name={data.title} />
			<nav class="flex flex-1 items-center gap-1 overflow-x-auto" aria-label="Admin">
				{#each nav as item (item.href)}
					<a
						href={item.href}
						class="flex items-center gap-2 rounded-md px-3 py-1.5 text-sm whitespace-nowrap {active(item.href) ? 'bg-muted font-medium' : 'text-muted-foreground hover:text-foreground'}"
						aria-current={active(item.href) ? 'page' : undefined}><item.icon class="size-4" />{item.label}</a
					>
				{/each}
			</nav>
			<div class="flex items-center gap-1">
				<Button variant="ghost" size="icon" href="/" target="_blank" aria-label="Open the status page"><ExternalLink /></Button>
				<Button variant="ghost" size="icon" onclick={toggleMode} aria-label="Toggle theme"><Moon /></Button>
				<form method="POST" action="/logout">
					<Button variant="ghost" size="icon" type="submit" aria-label="Sign out ({data.username})" title="Sign out {data.username}"><LogOut /></Button>
				</form>
			</div>
		</div>
	</header>
	<main class="mx-auto flex w-full max-w-6xl flex-1 flex-col gap-6 px-4 py-8">
		{@render children()}
	</main>
</div>
