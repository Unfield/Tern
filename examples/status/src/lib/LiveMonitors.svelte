<script lang="go" live>
import (
	"context"
	"slices"
	"time"

	"github.com/Unfield/Tern/goscript"

	"status/internal/app"
	"status/internal/store"
)

// Row is a monitor with its recent checks, as the dashboard shows it.
type Row struct {
	store.Monitor
	Beats  []store.Heartbeat `json:"beats"`
	Uptime float64           `json:"uptime"` // 24 hours
}

type Summary struct {
	Up, Down, Pending, Paused int
}

func Setup(c *goscript.Component) {
	// The WebSocket is open to anyone, so check like an endpoint would.
	if _, ok := c.Locals()["admin"].(app.Admin); !ok {
		c.Reject("Sign in to see the monitors.")
	}
	rows := goscript.State(c, "rows", load(c.Context()))
	goscript.Derived(c, "summary", func() Summary {
		var s Summary
		for _, r := range rows.Get() {
			switch r.Status {
			case store.Up:
				s.Up++
			case store.Down:
				s.Down++
			case store.Paused:
				s.Paused++
			default:
				s.Pending++
			}
		}
		return s
	})

	// Every check arrives from the monitor engine's hub. Only what changed
	// goes to the browser, as a patch of the rows.
	c.Go(func(ctx context.Context) {
		checks, err := app.Hub.Subscribe(ctx, "monitors")
		if err != nil {
			return
		}
		for ev := range checks {
			c.Do(func() {
				rows.Update(func(rs []Row) []Row {
					i := slices.IndexFunc(rs, func(r Row) bool { return r.ID == ev.Monitor.ID })
					if i < 0 {
						return load(ctx) // a monitor we don't know yet
					}
					out := slices.Clone(rs)
					r := out[i]
					r.Status, r.LastCheck, r.LastLatency, r.LastMessage = ev.Monitor.Status, ev.Monitor.LastCheck, ev.Monitor.LastLatency, ev.Beat.Message
					if ev.Beat.Status != store.Paused {
						r.Beats = append(slices.Clone(r.Beats), ev.Beat)
						r.Beats = r.Beats[max(0, len(r.Beats)-40):]
					}
					out[i] = r
					return out
				})
			})
		}
	})
	// The time relative times count from, pushed from the server.
	now := goscript.State(c, "now", time.Now().UnixMilli())
	c.Every(5*time.Second, func() { now.Set(time.Now().UnixMilli()) })
	// Uptime and monitors added elsewhere refresh every half minute.
	c.Every(30*time.Second, func() { rows.Set(load(c.Context())) })
}

func load(ctx context.Context) []Row {
	monitors, err := app.Store.Monitors(ctx)
	if err != nil {
		return []Row{}
	}
	beats, err := app.Store.RecentBeats(ctx, 40)
	if err != nil {
		return []Row{}
	}
	out := []Row{}
	for _, m := range monitors {
		u, err := app.Store.Uptime(ctx, m.ID, time.Now().Add(-24*time.Hour))
		if err != nil {
			continue
		}
		b := beats[m.ID]
		if b == nil {
			b = []store.Heartbeat{}
		}
		out = append(out, Row{Monitor: m, Beats: b, Uptime: u.Ratio})
	}
	return out
}
</script>

<script module lang="ts">
	// Go is the instance script; components and helpers the markup uses
	// are imported here.
	import Plus from '@lucide/svelte/icons/plus';
	import Beats from '$lib/Beats.svelte';
	import StatusDot from '$lib/StatusDot.svelte';
	import { Badge } from '$lib/components/ui/badge/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import * as Card from '$lib/components/ui/card/index.js';
	import { ago, ms, percent, statusLabel } from '$lib/format.js';
</script>

<div class="flex flex-wrap items-center justify-between gap-3">
	<div>
		<h1 class="text-2xl font-semibold tracking-tight">Monitors</h1>
		<p class="text-muted-foreground text-sm">
			Live: <span data-testid="live-status">{live.connected ? 'connected' : 'connecting'}</span>
		</p>
	</div>
	<Button href="/admin/monitors/new"><Plus /> New monitor</Button>
</div>

{#if live.error}
	<Card.Root><Card.Content class="py-8 text-center" data-testid="live-error">{live.error}</Card.Content></Card.Root>
{:else}
<div class="grid grid-cols-2 gap-4 md:grid-cols-4">
	{#each [
		{ label: 'Up', value: summary.Up, tone: 'text-emerald-600' },
		{ label: 'Down', value: summary.Down, tone: 'text-red-600' },
		{ label: 'Pending', value: summary.Pending, tone: 'text-amber-600' },
		{ label: 'Paused', value: summary.Paused, tone: 'text-muted-foreground' }
	] as s (s.label)}
		<Card.Root>
			<Card.Header class="pb-2"><Card.Description>{s.label}</Card.Description></Card.Header>
			<Card.Content><span class="text-3xl font-semibold {s.tone}" data-testid="count-{s.label.toLowerCase()}">{s.value}</span></Card.Content>
		</Card.Root>
	{/each}
</div>

{#if (rows ?? []).length === 0}
	<Card.Root>
		<Card.Content class="grid justify-items-center gap-3 py-12 text-center">
			<p class="font-medium">No monitors yet</p>
			<p class="text-muted-foreground text-sm">Add a website, API, port or domain to start checking it.</p>
			<Button href="/admin/monitors/new"><Plus /> Add your first monitor</Button>
		</Card.Content>
	</Card.Root>
{:else}
	<Card.Root>
		<Card.Content class="divide-y p-0" data-testid="monitors">
			{#each rows ?? [] as m (m.id)}
				<a href="/admin/monitors/{m.id}" class="hover:bg-muted/50 grid gap-3 p-4 md:grid-cols-[1fr_auto_auto] md:items-center" data-testid="monitor-row" data-name={m.name}>
					<div class="min-w-0">
						<div class="flex items-center gap-2 font-medium">
							<StatusDot status={m.status} pulse />
							<span class="truncate">{m.name}</span>
							<Badge variant="outline" class="uppercase">{m.type}</Badge>
						</div>
						<p class="text-muted-foreground mt-1 truncate text-xs">{m.target} · {m.lastMessage || 'not checked yet'}</p>
					</div>
					<Beats beats={m.beats ?? []} />
					<div class="flex items-center gap-4 text-sm md:w-56 md:justify-end">
						<span class="text-muted-foreground">{ms(m.lastLatency)}</span>
						<span>{percent(m.uptime)}</span>
						<Badge variant={m.status === 'down' ? 'destructive' : 'secondary'} data-testid="row-status">{statusLabel[m.status]}</Badge>
					</div>
					<span class="sr-only">checked {ago(m.lastCheck, now)}</span>
				</a>
			{/each}
		</Card.Content>
	</Card.Root>
{/if}
{/if}
