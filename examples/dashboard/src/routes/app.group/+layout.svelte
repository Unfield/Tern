<script lang="ts">
	import { i18n, navigating, page, setLocale, t } from '@ternkit/tern';
	import { toggleMode } from 'mode-watcher';
	import ChevronsUpDown from '@lucide/svelte/icons/chevrons-up-down';
	import FolderKanban from '@lucide/svelte/icons/folder-kanban';
	import LayoutDashboard from '@lucide/svelte/icons/layout-dashboard';
	import LogOut from '@lucide/svelte/icons/log-out';
	import Languages from '@lucide/svelte/icons/languages';
	import Moon from '@lucide/svelte/icons/moon';
	import Settings from '@lucide/svelte/icons/settings';
	import * as Avatar from '$lib/components/ui/avatar/index.js';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu/index.js';
	import { Separator } from '$lib/components/ui/separator/index.js';
	import * as Sidebar from '$lib/components/ui/sidebar/index.js';
	import { route } from '$routes';
	import type { LayoutProps } from './$types';

	let { data, children }: LayoutProps = $props();

	const nav = $derived([
		{ href: '/', label: t('nav.overview'), icon: LayoutDashboard },
		{ href: '/projects', label: t('nav.projects'), icon: FolderKanban },
		{ href: '/settings', label: t('nav.settings'), icon: Settings }
	]);
	const languages: Record<string, string> = { en: 'English', de: 'Deutsch' };

	const initials = $derived(
		data.displayName
			.split(/\s+/)
			.map((w) => w[0])
			.join('')
			.slice(0, 2)
			.toUpperCase()
	);
	const section = $derived(nav.find((n) => n.href !== '/' && page.url.pathname.startsWith(n.href)) ?? nav[0]!);
	let logoutForm: HTMLFormElement | undefined = $state();
</script>

{#if navigating.current}
	<div class="bg-primary fixed inset-x-0 top-0 z-50 h-0.5 animate-pulse" aria-hidden="true"></div>
{/if}

<Sidebar.Provider>
	<Sidebar.Root collapsible="icon">
		<Sidebar.Header>
			<Sidebar.Menu>
				<Sidebar.MenuItem>
					<Sidebar.MenuButton size="lg">
						{#snippet child({ props })}
							<a href="/" {...props}>
								<div class="bg-primary text-primary-foreground flex size-8 items-center justify-center rounded-lg font-bold">A</div>
								<div class="grid text-left text-sm leading-tight">
									<span class="font-semibold">Acme Inc.</span>
									<span class="text-muted-foreground text-xs">Dashboard</span>
								</div>
							</a>
						{/snippet}
					</Sidebar.MenuButton>
				</Sidebar.MenuItem>
			</Sidebar.Menu>
		</Sidebar.Header>

		<Sidebar.Content>
			<Sidebar.Group>
				<Sidebar.GroupLabel>{t('nav.navigation')}</Sidebar.GroupLabel>
				<Sidebar.GroupContent>
					<Sidebar.Menu>
						{#each nav as item (item.href)}
							<Sidebar.MenuItem>
								<Sidebar.MenuButton isActive={section.href === item.href} tooltipContent={item.label}>
									{#snippet child({ props })}
										<a href={item.href} {...props}><item.icon /><span>{item.label}</span></a>
									{/snippet}
								</Sidebar.MenuButton>
							</Sidebar.MenuItem>
						{/each}
					</Sidebar.Menu>
				</Sidebar.GroupContent>
			</Sidebar.Group>

			<Sidebar.Group>
				<Sidebar.GroupLabel>{t('nav.activeProjects')}</Sidebar.GroupLabel>
				<Sidebar.GroupContent>
					<Sidebar.Menu data-testid="sidebar-projects">
						{#each data.projects ?? [] as p (p.id)}
							<Sidebar.MenuItem>
								<Sidebar.MenuButton isActive={page.url.pathname === route('/projects/[id]', { id: p.id })}>
									{#snippet child({ props })}
										<a href={route('/projects/[id]', { id: p.id })} {...props}><span>{p.name}</span></a>
									{/snippet}
								</Sidebar.MenuButton>
							</Sidebar.MenuItem>
						{/each}
					</Sidebar.Menu>
				</Sidebar.GroupContent>
			</Sidebar.Group>
		</Sidebar.Content>

		<Sidebar.Footer>
			<Sidebar.Menu>
				<Sidebar.MenuItem>
					<DropdownMenu.Root>
						<DropdownMenu.Trigger>
							{#snippet child({ props })}
								<Sidebar.MenuButton size="lg" {...props} data-testid="user-menu">
									<Avatar.Root class="size-8 rounded-lg">
										<Avatar.Fallback class="rounded-lg">{initials}</Avatar.Fallback>
									</Avatar.Root>
									<div class="grid flex-1 text-left text-sm leading-tight">
										<span class="truncate font-medium" data-testid="user">{data.displayName}</span>
										<span class="text-muted-foreground truncate text-xs">{data.email || t('menu.noEmail')}</span>
									</div>
									<ChevronsUpDown class="ml-auto" />
								</Sidebar.MenuButton>
							{/snippet}
						</DropdownMenu.Trigger>
						<DropdownMenu.Content side="top" align="start" class="w-56">
							<DropdownMenu.Label>{t('menu.signedInAs', { user: data.user })}</DropdownMenu.Label>
							<DropdownMenu.Separator />
							<DropdownMenu.Item onSelect={toggleMode}><Moon /> {t('menu.theme')}</DropdownMenu.Item>
							<DropdownMenu.Sub>
								<DropdownMenu.SubTrigger data-testid="language"><Languages /> {t('menu.language')}</DropdownMenu.SubTrigger>
								<DropdownMenu.SubContent>
									<DropdownMenu.RadioGroup value={i18n.locale} onValueChange={(l) => setLocale(l)}>
										{#each i18n.locales as l (l)}
											<DropdownMenu.RadioItem value={l}>{languages[l] ?? l}</DropdownMenu.RadioItem>
										{/each}
									</DropdownMenu.RadioGroup>
								</DropdownMenu.SubContent>
							</DropdownMenu.Sub>
							<DropdownMenu.Item onSelect={() => logoutForm?.requestSubmit()}><LogOut /> {t('menu.signOut')}</DropdownMenu.Item>
						</DropdownMenu.Content>
					</DropdownMenu.Root>
				</Sidebar.MenuItem>
			</Sidebar.Menu>
			<form bind:this={logoutForm} method="POST" action="/logout" hidden></form>
		</Sidebar.Footer>
		<Sidebar.Rail />
	</Sidebar.Root>

	<Sidebar.Inset>
		<header class="flex h-14 shrink-0 items-center gap-2 border-b px-4">
			<Sidebar.Trigger class="-ml-1" />
			<Separator orientation="vertical" class="mr-2 data-[orientation=vertical]:h-4" />
			<span class="text-sm font-medium" data-testid="section">{section.label}</span>
		</header>
		<div class="flex flex-1 flex-col gap-6 p-6">
			{@render children()}
		</div>
	</Sidebar.Inset>
</Sidebar.Provider>
