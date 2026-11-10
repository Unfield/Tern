package monitor

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/realtime"

	"status/internal/store"
)

// Event is published after every check, on the "monitors" topic for the
// admin pages and, for public monitors, on "public" without private details.
type Event struct {
	Monitor PublicMonitor   `json:"monitor"`
	Beat    store.Heartbeat `json:"beat"`
	Target  string          `json:"target,omitempty"`
}

// PublicMonitor is what the public status page may show about a monitor.
type PublicMonitor struct {
	ID          int64        `json:"id"`
	Name        string       `json:"name"`
	Group       string       `json:"group"`
	Status      store.Status `json:"status"`
	LastCheck   *int64       `json:"lastCheck"`
	LastLatency *int         `json:"lastLatency"`
}

// Public returns the public view of m.
func Public(m store.Monitor) PublicMonitor {
	return PublicMonitor{ID: m.ID, Name: m.Name, Group: m.Group, Status: m.Status, LastCheck: m.LastCheck, LastLatency: m.LastLatency}
}

// Change is a monitor going down or coming back up, for notifications.
type Change struct {
	MonitorID int64         `json:"monitorId"`
	Name      string        `json:"name"`
	Target    string        `json:"target"`
	Status    store.Status  `json:"status"`
	Message   string        `json:"message"`
	At        time.Time     `json:"at"`
	Downtime  time.Duration `json:"downtime"` // for recoveries
}

// Engine runs every monitor's checks on its interval.
type Engine struct {
	Store *store.Store
	Hub   *realtime.Hub[Event]
	// Notify is called when a monitor goes down or comes back up.
	Notify func(ctx context.Context, c Change)
	Log    *slog.Logger

	mu      sync.Mutex
	ctx     context.Context
	loops   map[int64]context.CancelFunc
	fails   map[int64]int
	downAt  map[int64]time.Time
	checkMu sync.Map // per-monitor locks, so CheckNow and the loop don't overlap
}

// Start runs the checks until ctx ends.
func (e *Engine) Start(ctx context.Context) error {
	if e.Log == nil {
		e.Log = slog.Default()
	}
	monitors, err := e.Store.Monitors(ctx)
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.init()
	e.ctx = ctx
	for _, m := range monitors {
		if m.Status == store.Down {
			e.downAt[m.ID] = time.Now()
			e.fails[m.ID] = m.Retries + 1
		}
	}
	e.mu.Unlock()
	for _, m := range monitors {
		e.start(m)
	}
	return nil
}

// init creates the engine's maps; call it with e.mu held.
func (e *Engine) init() {
	if e.loops == nil {
		e.loops = map[int64]context.CancelFunc{}
		e.fails = map[int64]int{}
		e.downAt = map[int64]time.Time{}
	}
	if e.Log == nil {
		e.Log = slog.Default()
	}
}

func (e *Engine) start(m store.Monitor) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.init()
	if cancel, ok := e.loops[m.ID]; ok {
		cancel()
		delete(e.loops, m.ID)
	}
	if m.Paused || e.ctx == nil {
		return
	}
	ctx, cancel := context.WithCancel(e.ctx)
	e.loops[m.ID] = cancel
	go e.loop(ctx, m.ID, time.Duration(m.IntervalSeconds)*time.Second)
}

// Reload picks up a monitor's new settings, or stops it if it was deleted
// or paused. Call it after every change.
func (e *Engine) Reload(ctx context.Context, id int64) {
	m, err := e.Store.Monitor(ctx, id)
	if err != nil {
		e.mu.Lock()
		e.init()
		if cancel, ok := e.loops[id]; ok {
			cancel()
			delete(e.loops, id)
		}
		delete(e.fails, id)
		delete(e.downAt, id)
		e.mu.Unlock()
		return
	}
	if m.Paused {
		e.mu.Lock()
		e.init()
		delete(e.fails, id)
		delete(e.downAt, id)
		e.mu.Unlock()
		e.publish(m, store.Heartbeat{MonitorID: id, At: time.Now().UnixMilli(), Status: store.Paused})
	}
	e.start(m)
}

func (e *Engine) loop(ctx context.Context, id int64, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		e.CheckNow(ctx, id)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// CheckNow checks a monitor at once and records the result.
func (e *Engine) CheckNow(ctx context.Context, id int64) {
	lock, _ := e.checkMu.LoadOrStore(id, &sync.Mutex{})
	lock.(*sync.Mutex).Lock()
	defer lock.(*sync.Mutex).Unlock()

	m, err := e.Store.Monitor(ctx, id)
	if err != nil || m.Paused {
		return
	}
	r := Check(ctx, m)
	if ctx.Err() != nil {
		return // stopped mid-check: don't record a false failure
	}
	e.process(ctx, m, r)
}

func (e *Engine) process(ctx context.Context, m store.Monitor, r Result) {
	now := time.Now()
	e.mu.Lock()
	e.init()
	wasDown := m.Status == store.Down
	status := store.Up
	if r.OK {
		e.fails[m.ID] = 0
	} else {
		e.fails[m.ID]++
		status = store.Pending
		if e.fails[m.ID] > m.Retries || wasDown {
			status = store.Down
		}
	}
	var downtime time.Duration
	switch {
	case status == store.Down && !wasDown:
		e.downAt[m.ID] = now
	case status == store.Up && wasDown:
		if t, ok := e.downAt[m.ID]; ok {
			downtime = now.Sub(t)
		}
		delete(e.downAt, m.ID)
	}
	e.mu.Unlock()

	beat := store.Heartbeat{MonitorID: m.ID, At: now.UnixMilli(), Status: status, LatencyMS: int(r.Latency.Milliseconds()), Message: r.Message}
	var cert *int64
	if r.CertExpires != nil {
		v := r.CertExpires.UnixMilli()
		cert = &v
	}
	if err := e.Store.Record(ctx, beat, status, cert); err != nil {
		e.Log.Error("recording a check", "monitor", m.Name, "err", err)
		return
	}
	m.Status, m.LastCheck, m.LastMessage = status, &beat.At, beat.Message
	m.LastLatency = &beat.LatencyMS
	e.publish(m, beat)

	switch {
	case status == store.Down && !wasDown:
		e.wentDown(ctx, m, r)
	case status == store.Up && wasDown:
		e.cameBack(ctx, m, downtime)
	}
}

func (e *Engine) publish(m store.Monitor, beat store.Heartbeat) {
	if e.Hub == nil {
		return
	}
	pub := Public(m)
	if beat.Status == store.Paused {
		pub.Status = store.Paused
	}
	_ = e.Hub.Publish("monitors", Event{Monitor: pub, Beat: beat, Target: m.Target})
	if m.Public {
		_ = e.Hub.Publish("public", Event{Monitor: pub, Beat: beat})
	}
}

func (e *Engine) wentDown(ctx context.Context, m store.Monitor, r Result) {
	e.Log.Warn("monitor is down", "monitor", m.Name, "reason", r.Message)
	if m.Public {
		if _, open, err := e.Store.OpenAutoIncident(ctx, m.ID); err == nil && !open {
			body := fmt.Sprintf("Our checks found **%s** unavailable: %s. We are looking into it.", m.Name, r.Message)
			if _, err := e.Store.OpenIncident(ctx, m.Name+" is down", "major", body, &m.ID, true); err != nil {
				e.Log.Error("opening an incident", "err", err)
			}
		}
	}
	if e.Notify != nil {
		e.Notify(ctx, Change{MonitorID: m.ID, Name: m.Name, Target: m.Target, Status: store.Down, Message: r.Message, At: time.Now()})
	}
}

func (e *Engine) cameBack(ctx context.Context, m store.Monitor, downtime time.Duration) {
	e.Log.Info("monitor is back up", "monitor", m.Name, "downtime", downtime.Round(time.Second))
	if id, open, err := e.Store.OpenAutoIncident(ctx, m.ID); err == nil && open {
		body := fmt.Sprintf("**%s** is available again after %s.", m.Name, humanDuration(downtime))
		if err := e.Store.UpdateIncident(ctx, id, store.Resolved, body); err != nil {
			e.Log.Error("resolving an incident", "err", err)
		}
	}
	if e.Notify != nil {
		e.Notify(ctx, Change{MonitorID: m.ID, Name: m.Name, Target: m.Target, Status: store.Up, Message: m.LastMessage, At: time.Now(), Downtime: downtime})
	}
}

func humanDuration(d time.Duration) string {
	switch {
	case d <= 0:
		return "a short outage"
	case d < time.Minute:
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	default:
		return fmt.Sprintf("%.1f hours", d.Hours())
	}
}

// Name implements kit.Plugin.
func (e *Engine) Name() string { return "monitors" }

// Setup implements kit.Plugin: checks start with the server, after the
// migrations, and stop when it shuts down.
func (e *Engine) Setup(s *kit.Setup) error {
	s.OnStart(e.Start)
	return nil
}
