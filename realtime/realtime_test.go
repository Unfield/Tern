package realtime_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/realtime"
)

type update struct {
	Name string `json:"name"`
	Up   bool   `json:"up"`
}

func TestHub(t *testing.T) {
	broker := realtime.NewMemoryBroker()
	hub := realtime.NewHubWith[update](broker)
	ctx, cancel := context.WithCancel(context.Background())
	a, err := hub.Subscribe(ctx, "monitors")
	if err != nil {
		t.Fatal(err)
	}
	other, _ := hub.Subscribe(ctx, "other")

	_ = hub.Publish("monitors", update{Name: "api", Up: true})
	if got := <-a; got != (update{Name: "api", Up: true}) {
		t.Errorf("got %+v", got)
	}
	select {
	case v := <-other:
		t.Errorf("another topic received %+v", v)
	default:
	}

	cancel()
	if _, ok := <-a; ok {
		t.Error("the channel stays open after cancel")
	}
	deadline := time.Now().Add(time.Second)
	for broker.Subscribers("monitors") != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if n := broker.Subscribers("monitors"); n != 0 {
		t.Errorf("subscribers after cancel = %d", n)
	}
	_ = hub.Publish("monitors", update{}) // no subscribers: no panic
}

func TestSlowSubscribersMissMessages(t *testing.T) {
	hub := realtime.NewHub[int]()
	hub.Buffer = 2
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, _ := hub.Subscribe(ctx, "n")
	for i := range 10 {
		_ = hub.Publish("n", i) // never blocks
	}
	if got := []int{<-ch, <-ch}; got[0] != 0 || got[1] != 1 {
		t.Errorf("got %v", got)
	}
	select {
	case v := <-ch:
		t.Errorf("unexpected %d", v)
	default:
	}
}

func TestStream(t *testing.T) {
	hub := realtime.NewHub[update]()
	m := &kit.Manifest{
		AppHTML: `<html><head>%tern.head%</head><body>%tern.body%</body></html>`,
		Routes: []kit.RouteDef{{ID: "/events", Endpoint: map[string]kit.HandlerFunc{"GET": func(c *kit.Ctx) error {
			return realtime.Stream(c, hub, "monitors")
		}}}},
	}
	a, err := kit.New(m)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a)
	defer srv.Close()
	res, err := http.Get(srv.URL + "/events")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	go func() {
		for range 50 {
			_ = hub.Publish("monitors", update{Name: "db", Up: false})
			time.Sleep(10 * time.Millisecond)
		}
	}()
	r := bufio.NewReader(res.Body)
	var lines []string
	for len(lines) < 2 {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	if lines[0] != "event: monitors" || lines[1] != `data: {"name":"db","up":false}` {
		t.Errorf("lines = %q", lines)
	}
}
