package pgbroker_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Unfield/Tern/goscript"
	"github.com/Unfield/Tern/realtime/pgbroker"
)

var (
	color = goscript.Shared("color", "red")
	room  = goscript.Presence("room")
)

// TestCluster runs two processes of an "app" against one database and
// checks that live components in both see the same Shared value and
// Presence count.
func TestCluster(t *testing.T) {
	dsn := os.Getenv("TERN_TEST_POSTGRES")
	if os.Getenv("TERN_CLUSTER_NODE") != "" {
		clusterNode(dsn)
		return
	}
	if dsn == "" {
		t.Skip("TERN_TEST_POSTGRES is not set")
	}
	a := startNode(t, "a")
	a.waitFor(t, "online=1")
	b := startNode(t, "b")
	b.waitFor(t, "online=2")
	a.waitFor(t, "online=2")

	a.send("paint blue")
	b.waitFor(t, `color="blue"`)
	b.send("paint green")
	a.waitFor(t, `color="green"`)

	// A process that stops takes its visitor with it.
	b.stop()
	a.waitFor(t, "online=1")
}

type node struct {
	cmd   *exec.Cmd
	in    io.WriteCloser
	mu    sync.Mutex
	lines []string
	seen  chan struct{}
}

func startNode(t *testing.T, name string) *node {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestCluster$", "-test.v")
	cmd.Env = append(os.Environ(), "TERN_CLUSTER_NODE="+name)
	in, _ := cmd.StdinPipe()
	out, _ := cmd.StdoutPipe()
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	n := &node{cmd: cmd, in: in, seen: make(chan struct{}, 64)}
	go func() {
		s := bufio.NewScanner(out)
		for s.Scan() {
			n.mu.Lock()
			n.lines = append(n.lines, s.Text())
			n.mu.Unlock()
			select {
			case n.seen <- struct{}{}:
			default:
			}
		}
	}()
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	return n
}

func (n *node) send(line string) { _, _ = fmt.Fprintln(n.in, line) }

func (n *node) stop() {
	_ = n.in.Close()
	_ = n.cmd.Wait()
}

// waitFor waits until the node's latest state line contains want.
func (n *node) waitFor(t *testing.T, want string) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		n.mu.Lock()
		last := ""
		for i := len(n.lines) - 1; i >= 0; i-- {
			if strings.HasPrefix(n.lines[i], "state ") {
				last = n.lines[i]
				break
			}
		}
		n.mu.Unlock()
		if strings.Contains(last, want) {
			return
		}
		select {
		case <-n.seen:
		case <-time.After(50 * time.Millisecond):
		case <-deadline:
			t.Fatalf("timed out waiting for %q; last state: %q", want, last)
		}
	}
}

// clusterNode is one process: it mounts a component and follows
// commands on stdin until stdin closes.
func clusterNode(dsn string) {
	ctx := context.Background()
	b, err := pgbroker.New(ctx, dsn, pgbroker.Options{Channel: "tern_cluster_test"})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer b.Close()
	goscript.SetBroker(b)
	goscript.Register("room", func(c *goscript.Component) {
		color.Show(c, "color")
		room.Show(c, "online")
		room.Join(c)
		c.Func("paint", func(v string) { color.Set(v) })
	})
	var mu sync.Mutex
	state := map[string]string{}
	print := func() {
		fmt.Printf("state online=%s color=%s\n", state["online"], state["color"])
	}
	inst, err := goscript.MountLive(ctx, "room", goscript.LiveOptions{
		Emit:   func(name string, v []byte) { mu.Lock(); state[name] = string(v); mu.Unlock() },
		Commit: func() { mu.Lock(); print(); mu.Unlock() },
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	s := bufio.NewScanner(os.Stdin)
	for s.Scan() {
		if v, ok := strings.CutPrefix(s.Text(), "paint "); ok {
			_, _ = inst.Call(ctx, "paint", []byte(`["`+v+`"]`))
		}
	}
	inst.Destroy()
	goscript.SetBroker(nil) // says goodbye to the other processes
}
