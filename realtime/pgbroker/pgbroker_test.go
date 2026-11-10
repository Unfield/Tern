package pgbroker_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Unfield/Tern/goscript"
	"github.com/Unfield/Tern/realtime"
	"github.com/Unfield/Tern/realtime/pgbroker"
)

// Both brokers stand for two processes of one app.
var (
	_ realtime.Broker = (*pgbroker.Broker)(nil)
	_ goscript.Broker = (*pgbroker.Broker)(nil)
)

func TestBroker(t *testing.T) {
	dsn := os.Getenv("TERN_TEST_POSTGRES")
	if dsn == "" {
		t.Skip("TERN_TEST_POSTGRES is not set")
	}
	ctx := context.Background()
	opts := pgbroker.Options{Channel: "tern_test_" + strings.ReplaceAll(t.Name(), "/", "_")}
	a, err := pgbroker.New(ctx, dsn, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := pgbroker.New(ctx, dsn, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	got := make(chan string, 4)
	sctx, cancel := context.WithCancel(ctx)
	if err := b.Subscribe(sctx, "news", func(m []byte) { got <- string(m) }); err != nil {
		t.Fatal(err)
	}
	if err := a.Publish(ctx, "other", []byte("ignored")); err != nil {
		t.Fatal(err)
	}
	if err := a.Publish(ctx, "news", []byte(`{"n":1}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-got:
		if m != `{"n":1}` {
			t.Errorf("got %q", m)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no message")
	}
	cancel()
	time.Sleep(50 * time.Millisecond)
	_ = a.Publish(ctx, "news", []byte("after"))
	select {
	case m := <-got:
		t.Errorf("message after unsubscribing: %q", m)
	case <-time.After(300 * time.Millisecond):
	}
	if err := a.Publish(ctx, "news", make([]byte, 9000)); !errors.Is(err, pgbroker.ErrTooLarge) {
		t.Errorf("large message: %v", err)
	}
}
