<script lang="go" live>
import (
	"context"
	"time"

	"github.com/Unfield/Tern/goscript"

	"analytics/internal/app"
)

type Props struct {
	Domain string `json:"domain"`
}

func Setup(c *goscript.Component) {
	// The WebSocket is open to anyone, so check like an endpoint would.
	if _, ok := c.Locals()["user"].(app.User); !ok {
		c.Reject("Sign in to see live visitors.")
	}
	props := goscript.Props[Props](c)
	count := func(ctx context.Context) int {
		s, err := app.Store.SiteByDomain(ctx, props.Peek().Domain)
		if err != nil {
			return 0
		}
		n, _ := app.Store.Live(ctx, s.ID, time.Now())
		return n
	}
	now := goscript.State(c, "now", count(c.Context()))
	last := goscript.State(c, "last", int64(0))
	// Every page view of the site arrives here, from any server process.
	goscript.Subscribe(c, app.Topic(props.Peek().Domain), func(at int64) {
		now.Set(count(c.Context()))
		last.Set(at)
	})
	// Visitors also leave: count again now and then.
	c.Every(20*time.Second, func() { now.Set(count(c.Context())) })
}
</script>

<div class="live" data-testid="live">
	{#if live.error}
		<span>{live.error}</span>
	{:else}
		<span class="pulse" class:on={live.connected} aria-hidden="true"></span>
		<span role="status"><strong>{now ?? 0}</strong> {now === 1 ? 'visitor' : 'visitors'} right now</span>
	{/if}
</div>

<style>
	.live {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.35rem 0.75rem;
		font-size: 0.875rem;
		color: var(--tern-muted);
		background: var(--tern-surface);
		border: 1px solid var(--tern-border);
		border-radius: 2rem;
	}
	strong {
		color: var(--tern-fg);
		font-variant-numeric: tabular-nums;
	}
	.pulse {
		width: 0.5rem;
		height: 0.5rem;
		border-radius: 50%;
		background: var(--tern-border-strong);
	}
	.pulse.on {
		background: var(--tern-success);
		box-shadow: 0 0 0 0 color-mix(in oklch, var(--tern-success) 50%, transparent);
		animation: ping 2s infinite;
	}
	@keyframes ping {
		70% {
			box-shadow: 0 0 0 6px transparent;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.pulse.on {
			animation: none;
		}
	}
</style>
