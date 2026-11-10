<script lang="go" live>
import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/Unfield/Tern/goscript"
)

type Todo struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
	Done bool   `json:"done"`
}

// One list for every visitor. A real app keeps it in a database and uses
// goscript.Publish to tell the other components to reload.
var todos = goscript.Global([]Todo{})

func Setup(c *goscript.Component) {
	list := todos.Show(c, "todos")
	// Bind: the input sets it with bind:value={live.bind.draft}.
	draft := goscript.State(c, "draft", "", goscript.Bind)
	goscript.Derived(c, "remaining", func() int {
		n := 0
		for _, t := range list.Get() {
			if !t.Done {
				n++
			}
		}
		return n
	})
	goscript.Derived(c, "canAdd", func() bool { return strings.TrimSpace(draft.Get()) != "" })

	// A clock pushed from the server.
	now := goscript.State(c, "now", clock())
	c.Every(time.Second, func() { now.Set(clock()) })

	c.Func("add", func(ctx context.Context) error {
		text := strings.TrimSpace(draft.Peek())
		switch {
		case text == "":
			return goscript.Fail("Write something first.")
		case len(text) > 60:
			return goscript.Fail("Keep it under 60 characters.")
		}
		// Stands in for a database write, so the button shows it's busy.
		select {
		case <-time.After(150 * time.Millisecond):
		case <-ctx.Done():
			return ctx.Err()
		}
		todos.Update(func(ts []Todo) []Todo {
			id := 1
			for _, t := range ts {
				id = max(id, t.ID+1)
			}
			return append(slices.Clone(ts), Todo{ID: id, Text: text})
		})
		draft.Set("")
		c.Emit("added", text)
		return nil
	})
	c.Func("toggle", func(id int) {
		todos.Update(func(ts []Todo) []Todo {
			out := slices.Clone(ts)
			for i := range out {
				if out[i].ID == id {
					out[i].Done = !out[i].Done
				}
			}
			return out
		})
	})
	c.Func("clear", func() {
		todos.Update(func(ts []Todo) []Todo {
			return slices.DeleteFunc(slices.Clone(ts), func(t Todo) bool { return t.Done })
		})
	})
}

func clock() string { return time.Now().UTC().Format(time.TimeOnly) }
</script>

<div class="card" data-testid="todos">
	<h3>Live todos <small>server time <span data-testid="now">{now}</span> UTC</small></h3>
	<form
		onsubmit={(e) => {
			e.preventDefault();
			add();
		}}
	>
		<input bind:value={live.bind.draft} placeholder="Something to do" aria-label="New todo" data-testid="draft" />
		<button disabled={!canAdd || add.pending}>{add.pending ? 'Adding…' : 'Add'}</button>
	</form>
	{#if add.error}<p class="error" role="alert" data-testid="error">{add.error}</p>{/if}
	<ul>
		{#each todos as t (t.id)}
			<li>
				<label>
					<input
						type="checkbox"
						checked={t.done}
						onchange={() => toggle.optimistic({ todos: (todos ?? []).map((x) => (x.id === t.id ? { ...x, done: !x.done } : x)) }, t.id)}
					/>
					<span class:done={t.done}>{t.text}</span>
				</label>
			</li>
		{/each}
	</ul>
	<p>
		<span data-testid="remaining">{remaining}</span> left
		<button onclick={clear}>Clear done</button>
		{#if !live.connected}<em data-testid="offline">reconnecting…</em>{/if}
	</p>
</div>

<style>
	.done {
		text-decoration: line-through;
		opacity: 0.6;
	}
	.error {
		color: #b42318;
	}
	small {
		font-weight: normal;
		opacity: 0.7;
	}
</style>
