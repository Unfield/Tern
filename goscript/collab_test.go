package goscript

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

type here struct {
	Name  string `json:"name"`
	Field string `json:"field,omitempty"`
}

func TestRoom(t *testing.T) {
	b := &fakeBroker{}
	boards := Room[here]("board-test")
	SetBroker(b)
	t.Cleanup(func() { SetBroker(nil) })

	Register("board-test", func(c *Component) {
		var props struct {
			Board string `json:"board"`
			Name  string `json:"name"`
		}
		_ = c.Props(&props)
		key := "board:" + props.Board
		me := boards.Join(c, key, here{Name: props.Name})
		boards.Show(c, key, "people")
		State(c, "me", me.ID())
		c.Func("focus", func(field string) { me.Update(func(h here) here { h.Field = field; return h }) })
	})
	names := func(r *recorder) string {
		var people []Member[here]
		_ = json.Unmarshal([]byte(r.get("people")), &people)
		var out []string
		for _, p := range people {
			out = append(out, p.State.Name+":"+p.State.Field)
		}
		return strings.Join(out, ",")
	}

	ada, bob, eve := newRecorder(), newRecorder(), newRecorder()
	a := mustMount(t, "board-test", ada.opts(`{"board":"1","name":"Ada"}`, nil))
	time.Sleep(2 * time.Millisecond) // so Bob joins after Ada
	mustMount(t, "board-test", bob.opts(`{"board":"1","name":"Bob"}`, nil))
	mustMount(t, "board-test", eve.opts(`{"board":"2","name":"Eve"}`, nil))
	eventually(t, "Ada and Bob on board 1", func() bool { return names(ada) == "Ada:,Bob:" && names(bob) == "Ada:,Bob:" })
	if names(eve) != "Eve:" {
		t.Errorf("board 2 = %q", names(eve))
	}

	// A member's own state reaches everyone in the room.
	if _, err := a.Call(context.Background(), "focus", []byte(`["title"]`)); err != nil {
		t.Fatal(err)
	}
	eventually(t, "Ada's focus", func() bool { return names(bob) == "Ada:title,Bob:" })

	// Other processes' members join the room, and their updates are throttled into few messages.
	b.remote(topicPrefix+"room.board-test", envelope{Data: json.RawMessage(`{"k":"board:1","m":[{"id":"r1","state":{"name":"Cy"},"since":"2000-01-01T00:00:00Z"}]}`)})
	eventually(t, "Cy from afar, first by joining", func() bool { return names(ada) == "Cy:,Ada:title,Bob:" })
	sent := func() int {
		b.mu.Lock()
		defer b.mu.Unlock()
		n := 0
		for _, m := range b.sent {
			if strings.HasPrefix(m, topicPrefix+"room.board-test ") && strings.Contains(m, "board:1") && !strings.Contains(m, `"zzzz-other"`) {
				n++
			}
		}
		return n
	}
	before := sent()
	for i := range 20 {
		if _, err := a.Call(context.Background(), "focus", []byte(`["f`+string(rune('a'+i))+`"]`)); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(150 * time.Millisecond)
	if n := sent() - before; n == 0 || n > 4 {
		t.Errorf("20 quick updates sent %d messages", n)
	}

	// Leaving, here and elsewhere.
	a.Destroy()
	eventually(t, "Ada left", func() bool { return names(bob) == "Cy:,Bob:" })
	b.remote(topicPrefix+"room.board-test", envelope{Data: json.RawMessage(`{"k":"board:1","m":[]}`)})
	eventually(t, "Cy left", func() bool { return names(bob) == "Bob:" })
	if got := boards.Members("board:1"); len(got) != 1 || got[0].State.Name != "Bob" {
		t.Errorf("Members = %+v", got)
	}
}

type note struct {
	Title string   `json:"title"`
	Body  string   `json:"body"`
	Tags  []string `json:"tags"`
}

func TestDocs(t *testing.T) {
	b := &fakeBroker{}
	var mu sync.Mutex
	var saved []note
	loads := 0
	notes := Docs("note-test", DocOptions[note]{
		Load: func(_ context.Context, key string) (note, error) {
			mu.Lock()
			loads++
			mu.Unlock()
			return note{Title: "Draft", Body: "Hello"}, nil
		},
		Save: func(_ context.Context, key string, v note) error {
			mu.Lock()
			saved = append(saved, v)
			mu.Unlock()
			return nil
		},
		SaveEvery: 20 * time.Millisecond,
	})
	SetBroker(b)
	t.Cleanup(func() { SetBroker(nil) })

	Register("note-test", func(c *Component) {
		n := notes.Open(c, "note:1")
		n.Show(c, "note")
		c.Func("title", func(s string) error { return n.Update(func(v note) note { v.Title = s; return v }) })
		c.Func("body", func(s string) error { return n.Update(func(v note) note { v.Body = s; return v }) })
	})
	get := func(r *recorder) note {
		var n note
		_ = json.Unmarshal([]byte(r.get("note")), &n)
		return n
	}
	one, two := newRecorder(), newRecorder()
	a := mustMount(t, "note-test", one.opts("", nil))
	mustMount(t, "note-test", two.opts("", nil))
	eventually(t, "the loaded note", func() bool { return get(two).Title == "Draft" })
	if loads != 1 {
		t.Errorf("loaded %d times for two components", loads)
	}

	// A change reaches every component, and only its field goes out.
	if _, err := a.Call(context.Background(), "title", []byte(`["Plans"]`)); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the new title", func() bool { return get(two).Title == "Plans" })
	var change string
	eventually(t, "the change to be published", func() bool {
		b.mu.Lock()
		defer b.mu.Unlock()
		for _, m := range b.sent {
			if strings.HasPrefix(m, topicPrefix+"doc.note-test ") && strings.Contains(m, "Plans") {
				change = m
				return true
			}
		}
		return false
	})
	if strings.Contains(change, "Hello") || strings.Contains(change, `"body"`) {
		t.Errorf("the change sent other fields: %s", change)
	}

	// Another process changed the body meanwhile: both changes are kept.
	b.remote(topicPrefix+"doc.note-test", envelope{Data: json.RawMessage(`{"k":"note:1","f":{"body":{"v":"From afar","s":{"c":100,"o":"zzzz-other"}}}}`)})
	eventually(t, "the merged note", func() bool { n := get(one); return n.Title == "Plans" && n.Body == "From afar" })
	// Clocks follow what they've seen: a local change now is later than the
	// remote one, and a remote change older than it loses.
	if _, err := a.Call(context.Background(), "title", []byte(`["Plans 2"]`)); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the second title", func() bool { return get(two).Title == "Plans 2" })
	b.remote(topicPrefix+"doc.note-test", envelope{Data: json.RawMessage(`{"k":"note:1","f":{"title":{"v":"Old","s":{"c":101,"o":"zzzz-other"}}}}`)})
	time.Sleep(20 * time.Millisecond)
	if get(one).Title != "Plans 2" {
		t.Errorf("an older change won: %+v", get(one))
	}
	if _, err := a.Call(context.Background(), "body", []byte(`["Mine"]`)); err != nil {
		t.Fatal(err)
	}
	eventually(t, "a local change after a remote one", func() bool { return get(two).Body == "Mine" })

	// A process that opens it asks, and gets the changed fields, not the loaded ones.
	b.mu.Lock()
	b.sent = nil
	b.mu.Unlock()
	b.remote(topicPrefix+"doc.note-test", envelope{Hello: true, Data: json.RawMessage(`{"k":"note:1"}`)})
	eventually(t, "a reply to the hello", func() bool {
		b.mu.Lock()
		defer b.mu.Unlock()
		for _, m := range b.sent {
			if !strings.Contains(m, `"h":true`) && strings.Contains(m, "Plans") && strings.Contains(m, "Mine") {
				return true
			}
		}
		return false
	})

	// Saves gather changes.
	eventually(t, "a save", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(saved) > 0 && saved[len(saved)-1].Title == "Plans 2" && saved[len(saved)-1].Body == "Mine"
	})
	mu.Lock()
	if len(saved) > 3 {
		t.Errorf("saved %d times", len(saved))
	}
	mu.Unlock()
}

func TestDocsMustBeObjects(t *testing.T) {
	SetBroker(nil)
	nums := Docs("nums-test", DocOptions[int]{Load: func(context.Context, string) (int, error) { return 1, nil }})
	Register("nums-test", func(c *Component) { nums.Open(c, "x") })
	r := newRecorder()
	if _, err := MountLive(context.Background(), "nums-test", r.opts("", nil)); err == nil {
		t.Error("a document that isn't an object was opened")
	}
}
