<script lang="go" live>
import (
	"hash/fnv"
	"strings"

	"github.com/Unfield/Tern/goscript"
)

// Note is the shared document: each field merges on its own, so two
// people typing in different fields never undo each other.
type Note struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// Here is what each person in the room shares: who they are, and the
// field they're in.
type Here struct {
	Name  string `json:"name"`
	Color string `json:"color"`
	Field string `json:"field"`
}

var (
	notes = goscript.Docs("note", goscript.DocOptions[Note]{})
	room  = goscript.Room[Here]("note")
)

var colors = []string{"#5ec4b6", "#f59e0b", "#a78bfa", "#f472b6", "#60a5fa", "#4ade80"}

// colorOf gives each name its own color.
func colorOf(name string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return colors[h.Sum32()%uint32(len(colors))]
}

func Setup(c *goscript.Component) {
	var props struct {
		Name string `json:"name"`
	}
	_ = c.Props(&props)
	const key = "note:demo"

	note := notes.Open(c, key)
	note.Show(c, "note")
	room.Show(c, key, "people")

	me := room.Join(c, key, Here{Name: props.Name, Color: colorOf(props.Name)})
	goscript.State(c, "me", me.ID())

	c.Func("edit", func(field, value string) {
		_ = note.Update(func(n Note) Note {
			switch field {
			case "title":
				n.Title = strings.TrimSpace(value)
			case "body":
				n.Body = value
			}
			return n
		})
	})
	c.Func("focus", func(field string) { me.Update(func(h Here) Here { h.Field = field; return h }) })
}
</script>

<div class="card note" data-testid="note">
	<h3>Live: a shared note</h3>
	<ul class="people" aria-label="Here now">
		{#each people as p (p.id)}
			<li style="--c: {p.state.color}" title={p.state.field ? `${p.state.name} is editing the ${p.state.field}` : p.state.name}>
				<span class="dot"></span>{p.state.name}{#if p.id === me}&nbsp;(you){/if}{#if p.state.field}<span class="where">&nbsp;· {p.state.field}</span>{/if}
			</li>
		{/each}
	</ul>
	{#snippet editing(field: string)}
		{#each people.filter((p) => p.id !== me && p.state.field === field) as p (p.id)}
			<span class="tag" style="--c: {p.state.color}">{p.state.name}</span>
		{/each}
	{/snippet}
	<label>
		<span>Title {@render editing('title')}</span>
		<input data-testid="note-title" value={note.title} oninput={(e) => edit.debounce(150)('title', e.currentTarget.value)} onfocus={() => focus('title')} onblur={() => focus('')} />
	</label>
	<label>
		<span>Body {@render editing('body')}</span>
		<textarea data-testid="note-body" rows="4" value={note.body} oninput={(e) => edit.debounce(150)('body', e.currentTarget.value)} onfocus={() => focus('body')} onblur={() => focus('')}></textarea>
	</label>
</div>

<style>
	.note label {
		display: grid;
		gap: 0.25rem;
		margin-top: 0.75rem;
	}
	.note input,
	.note textarea {
		font: inherit;
		padding: 0.4rem 0.6rem;
	}
	.people {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem 1rem;
		list-style: none;
		margin: 0;
		padding: 0;
		font-size: 0.875rem;
	}
	.dot {
		display: inline-block;
		width: 0.6rem;
		height: 0.6rem;
		margin-right: 0.35rem;
		border-radius: 50%;
		background: var(--c);
	}
	.where {
		opacity: 0.7;
	}
	.tag {
		margin-left: 0.4rem;
		padding: 0 0.4rem;
		border-radius: 4px;
		background: var(--c);
		color: #111;
		font-size: 0.75rem;
	}
</style>
