package billing_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Unfield/Tern/billing"
	"github.com/Unfield/Tern/internal/dbtest"
	"github.com/Unfield/Tern/jobs"
	"github.com/Unfield/Tern/kit"
)

const webhookSecret = "whsec_test"

// fakeStripe answers the API calls billing makes, keeping subscriptions
// in memory.
type fakeStripe struct {
	mu        sync.Mutex
	srv       *httptest.Server
	customers map[string]string // idempotency key → id
	subs      map[string]map[string]any
	checkouts []map[string]string
	calls     []string
}

func newFakeStripe(t *testing.T) *fakeStripe {
	f := &fakeStripe{customers: map[string]string{}, subs: map[string]map[string]any{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeStripe) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	if r.Header.Get("Authorization") != "Bearer sk_test" {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"type":"invalid_request_error","message":"Invalid API Key"}}`)
		return
	}
	_ = r.ParseForm()
	form := map[string]string{}
	for k := range r.PostForm {
		form[k] = r.PostForm.Get(k)
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/v1/customers":
		key := r.Header.Get("Idempotency-Key")
		id, ok := f.customers[key]
		if !ok {
			id = fmt.Sprintf("cus_%d", time.Now().UnixNano())
			f.customers[key] = id
		}
		writeJSON(w, map[string]any{"id": id})
	case r.Method == http.MethodPost && r.URL.Path == "/v1/checkout/sessions":
		f.checkouts = append(f.checkouts, form)
		writeJSON(w, map[string]any{"id": "cs_1", "url": "https://checkout.stripe.test/cs_1"})
	case r.Method == http.MethodPost && r.URL.Path == "/v1/billing_portal/sessions":
		writeJSON(w, map[string]any{"url": "https://billing.stripe.test/p_1?customer=" + form["customer"]})
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/subscriptions/"):
		s, ok := f.subs[strings.TrimPrefix(r.URL.Path, "/v1/subscriptions/")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":{"message":"No such subscription"}}`)
			return
		}
		writeJSON(w, s)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/subscription_items/"):
		for _, s := range f.subs {
			item := s["items"].(map[string]any)["data"].([]any)[0].(map[string]any)
			if item["id"] == strings.TrimPrefix(r.URL.Path, "/v1/subscription_items/") {
				item["quantity"], _ = strconv.Atoi(form["quantity"])
			}
		}
		writeJSON(w, map[string]any{})
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// subscribe makes a subscription, as completing Checkout does.
func (f *fakeStripe) subscribe(id, customer, owner, price, status string, quantity int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.subs[id] = map[string]any{
		"id": id, "object": "subscription", "customer": customer, "status": status, "metadata": map[string]any{"owner": owner},
		"cancel_at_period_end": false, "trial_end": nil,
		"items": map[string]any{"data": []any{map[string]any{
			"id": "si_" + id, "quantity": quantity, "current_period_end": time.Now().Add(30 * 24 * time.Hour).Unix(),
			"price": map[string]any{"id": price, "recurring": map[string]any{"interval": "month"}},
		}}},
	}
}

func (f *fakeStripe) set(id, key string, v any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.subs[id][key] = v
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

var plans = []billing.Plan{
	{ID: "free", Name: "Free", Limits: map[string]int{"projects": 2}},
	{ID: "pro", Name: "Pro", Prices: map[billing.Interval]string{billing.Monthly: "price_pro_m", billing.Yearly: "price_pro_y"}, TrialDays: 14,
		Limits: map[string]int{"projects": 50}, Features: []string{"export"}},
	{ID: "team", Name: "Team", Prices: map[billing.Interval]string{billing.Monthly: "price_team_m"}, PerSeat: true},
}

type harness struct {
	t      *testing.T
	b      *billing.Billing
	stripe *fakeStripe
	srv    *httptest.Server
	q      *jobs.Queue
}

func newHarness(t *testing.T, db *sql.DB, withJobs bool) *harness {
	t.Helper()
	h := &harness{t: t, stripe: newFakeStripe(t)}
	opts := billing.Options{SecretKey: "sk_test", WebhookSecret: webhookSecret, Plans: plans, APIBase: h.stripe.srv.URL}
	if withJobs {
		q, err := jobs.New(db, jobs.Options{PollInterval: 20 * time.Millisecond, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
		if err != nil {
			t.Fatal(err)
		}
		if err := q.Migrate(context.Background()); err != nil {
			t.Fatal(err)
		}
		opts.Jobs, h.q = q, q
	}
	b, err := billing.New(db, opts)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.b = b
	if h.q != nil {
		if err := h.q.Start(context.Background()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = h.q.Stop(context.Background()) })
	}
	m := &kit.Manifest{AppHTML: `<html><head>%tern.head%</head><body>%tern.body%</body></html>`, Routes: []kit.RouteDef{
		{ID: "/checkout", Endpoint: map[string]kit.HandlerFunc{"POST": func(c *kit.Ctx) error {
			seats, _ := strconv.Atoi(c.Request.FormValue("seats"))
			u, err := b.Checkout(c, billing.CheckoutOptions{Owner: c.Request.FormValue("owner"), Plan: c.Request.FormValue("plan"), Interval: billing.Interval(c.Request.FormValue("interval")),
				Email: "ada@example.com", Seats: seats, SuccessURL: "/billing?done=1", CancelURL: "/pricing"})
			if err != nil {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			return c.JSON(http.StatusOK, map[string]string{"url": u})
		}}},
		{ID: "/portal", Endpoint: map[string]kit.HandlerFunc{"POST": func(c *kit.Ctx) error {
			u, err := b.Portal(c, c.Request.FormValue("owner"), "/billing")
			if err != nil {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
			}
			return c.JSON(http.StatusOK, map[string]string{"url": u})
		}}},
	}}
	app, err := kit.New(m, kit.WithPlugins(b), kit.WithLogger(slog.New(slog.NewTextHandler(io.Discard, nil))))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	h.srv = httptest.NewServer(app)
	t.Cleanup(h.srv.Close)
	return h
}

func (h *harness) post(path string, kv ...string) (int, map[string]any) {
	h.t.Helper()
	form := make([]string, 0, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		form = append(form, kv[i]+"="+kv[i+1])
	}
	resp, err := http.Post(h.srv.URL+path, "application/x-www-form-urlencoded", strings.NewReader(strings.Join(form, "&")))
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// sign signs an event as Stripe does.
func sign(payload []byte, at time.Time) string {
	ts := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(webhookSecret))
	mac.Write([]byte(ts + "." + string(payload)))
	return "t=" + ts + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}

func (h *harness) event(id, typ string, object map[string]any) int {
	h.t.Helper()
	payload, _ := json.Marshal(map[string]any{"id": id, "type": typ, "data": map[string]any{"object": object}})
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/_tern/billing/webhook", strings.NewReader(string(payload)))
	req.Header.Set("Stripe-Signature", sign(payload, time.Now()))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

// waitFor waits for the owner's subscription to have status.
func (h *harness) waitFor(owner string, status billing.Status) *billing.Subscription {
	h.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		s, err := h.b.Subscription(context.Background(), owner)
		if err != nil {
			h.t.Fatal(err)
		}
		if s != nil && s.Status == status {
			return s
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("%s never became %s: %+v", owner, status, s)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestSubscribing(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *sql.DB) {
		for _, withJobs := range []bool{false, true} {
			t.Run(fmt.Sprintf("jobs=%v", withJobs), func(t *testing.T) {
				testSubscribing(t, db, withJobs)
			})
		}
	})
}

func testSubscribing(t *testing.T, db *sql.DB, withJobs bool) {
	h := newHarness(t, db, withJobs)
	ctx := context.Background()
	owner := billing.Team(time.Now().UnixNano())

	// Without a subscription: the free plan and its limits.
	if p, s, err := h.b.Current(ctx, owner); err != nil || p.ID != "free" || s != nil {
		t.Fatalf("before: %v %v %v", p.ID, s, err)
	}
	if err := h.b.Check(ctx, owner, "projects", 2); !errors.Is(err, billing.ErrLimit) {
		t.Fatalf("free plan's limit: %v", err)
	}
	if err := h.b.Check(ctx, owner, "projects", 1); err != nil {
		t.Fatal(err)
	}
	if err := h.b.Check(ctx, owner, "anything else", 1000); err != nil {
		t.Fatalf("a limit the plan doesn't list is unlimited: %v", err)
	}

	// Checkout: a customer made once, the trial for a first subscription.
	status, out := h.post("/checkout", "owner", owner, "plan", "pro", "interval", "month")
	if status != 200 || out["url"] != "https://checkout.stripe.test/cs_1" {
		t.Fatalf("checkout: %d %v", status, out)
	}
	co := h.stripe.checkouts[len(h.stripe.checkouts)-1]
	if co["line_items[0][price]"] != "price_pro_m" || co["subscription_data[trial_period_days]"] != "14" || co["subscription_data[metadata][owner]"] != owner ||
		!strings.HasSuffix(co["success_url"], "/billing?done=1") || !strings.HasPrefix(co["success_url"], "http://127.0.0.1") || co["customer"] == "" {
		t.Fatalf("checkout params %v", co)
	}
	customer := co["customer"]
	if status, out := h.post("/checkout", "owner", owner, "plan", "pro", "interval", "week"); status != 400 || !strings.Contains(out["error"].(string), "unknown plan") {
		t.Fatalf("a bad interval: %d %v", status, out)
	}
	h.post("/checkout", "owner", owner, "plan", "pro", "interval", "year")
	if co := h.stripe.checkouts[len(h.stripe.checkouts)-1]; co["customer"] != customer || co["subscription_data[trial_period_days]"] != "" {
		t.Fatalf("a second checkout made another customer or another trial: %v", co)
	}

	// The webhook: signed, once per event, and the subscription refetched.
	sub := "sub_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	h.stripe.subscribe(sub, customer, owner, "price_pro_m", "trialing", 1)
	if code := h.event("evt_"+sub+"_1", "checkout.session.completed", map[string]any{"object": "checkout.session", "id": "cs_1", "subscription": sub}); code != 200 {
		t.Fatalf("webhook: %d", code)
	}
	s := h.waitFor(owner, billing.Trialing)
	if s.Plan != "pro" || s.Interval != billing.Monthly || s.Seats != 1 || s.CurrentPeriodEnd.Before(time.Now()) {
		t.Fatalf("stored %+v", s)
	}
	if p, _, _ := h.b.Current(ctx, owner); p.ID != "pro" {
		t.Fatalf("plan during the trial: %s", p.ID)
	}
	if ok, _ := h.b.Has(ctx, owner, "export"); !ok {
		t.Error("pro includes export")
	}
	if err := h.b.Check(ctx, owner, "projects", 2); err != nil {
		t.Fatalf("pro's limit: %v", err)
	}

	// Payment fails: the plan still applies while Stripe retries.
	h.stripe.set(sub, "status", "past_due")
	h.event("evt_"+sub+"_2", "invoice.payment_failed", map[string]any{"object": "invoice", "id": "in_1", "parent": map[string]any{"subscription_details": map[string]any{"subscription": sub}}})
	h.waitFor(owner, billing.PastDue)
	if p, _, _ := h.b.Current(ctx, owner); p.ID != "pro" {
		t.Fatalf("plan while past due: %s", p.ID)
	}

	// Canceled: back to free. A repeated event changes nothing.
	h.stripe.set(sub, "status", "canceled")
	h.event("evt_"+sub+"_3", "customer.subscription.deleted", map[string]any{"object": "subscription", "id": sub})
	h.waitFor(owner, billing.Canceled)
	h.stripe.set(sub, "status", "active")
	h.event("evt_"+sub+"_3", "customer.subscription.deleted", map[string]any{"object": "subscription", "id": sub})
	time.Sleep(100 * time.Millisecond)
	if s, _ := h.b.Subscription(ctx, owner); s.Status != billing.Canceled {
		t.Fatalf("a repeated event was processed again: %s", s.Status)
	}
	if p, _, _ := h.b.Current(ctx, owner); p.ID != "free" {
		t.Fatalf("plan after canceling: %s", p.ID)
	}

	// The portal, for owners who have a customer.
	if status, out := h.post("/portal", "owner", owner); status != 200 || !strings.Contains(out["url"].(string), customer) {
		t.Fatalf("portal: %d %v", status, out)
	}
	if status, out := h.post("/portal", "owner", "team:nobody"); status != 400 || !strings.Contains(out["error"].(string), "never subscribed") {
		t.Fatalf("portal without a customer: %d %v", status, out)
	}
}

func TestSeats(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, db *sql.DB) {
		h := newHarness(t, db, false)
		ctx := context.Background()
		owner := billing.Team(7)
		h.post("/checkout", "owner", owner, "plan", "team", "interval", "month", "seats", "3")
		co := h.stripe.checkouts[0]
		if co["line_items[0][quantity]"] != "3" {
			t.Fatalf("seats at checkout: %v", co)
		}
		h.stripe.subscribe("sub_seats", co["customer"], owner, "price_team_m", "active", 3)
		h.event("evt_seats", "customer.subscription.created", map[string]any{"object": "subscription", "id": "sub_seats"})
		if s := h.waitFor(owner, billing.Active); s.Seats != 3 {
			t.Fatalf("seats %d", s.Seats)
		}
		if err := h.b.SetSeats(ctx, owner, 5); err != nil {
			t.Fatal(err)
		}
		if s, _ := h.b.Subscription(ctx, owner); s.Seats != 5 {
			t.Fatalf("after SetSeats: %d", s.Seats)
		}
	})
}

func TestWebhookSignature(t *testing.T) {
	payload := []byte(`{"id":"evt_1"}`)
	now := time.Now()
	if err := billing.VerifySignature(payload, sign(payload, now), webhookSecret, now); err != nil {
		t.Fatal(err)
	}
	for name, header := range map[string]string{
		"another payload's": sign([]byte(`{"id":"evt_2"}`), now),
		"too old":           sign(payload, now.Add(-10*time.Minute)),
		"empty":             "",
		"no v1":             "t=" + strconv.FormatInt(now.Unix(), 10),
	} {
		if err := billing.VerifySignature(payload, header, webhookSecret, now); !errors.Is(err, billing.ErrSignature) {
			t.Errorf("%s signature: %v", name, err)
		}
	}
	if err := billing.VerifySignature(payload, sign(payload, now)+",v1=00", webhookSecret, now); err != nil {
		t.Errorf("one good signature among several: %v", err)
	}

	db, err := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer func() { _ = db.Close() }()
	h := newHarness(t, db, false)
	req, _ := http.NewRequest(http.MethodPost, h.srv.URL+"/_tern/billing/webhook", strings.NewReader(string(payload)))
	req.Header.Set("Stripe-Signature", sign([]byte("other"), now))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("a forged webhook: %d", resp.StatusCode)
	}
}

func TestPlans(t *testing.T) {
	db, _ := sql.Open("sqlite", "file:"+t.Name()+"?mode=memory")
	defer func() { _ = db.Close() }()
	if _, err := billing.New(db, billing.Options{Plans: []billing.Plan{{ID: "a"}, {ID: "a"}}}); err == nil {
		t.Error("a plan defined twice")
	}
	if _, err := billing.New(db, billing.Options{Plans: []billing.Plan{{ID: "a", Prices: map[billing.Interval]string{"week": "p"}}}}); err == nil {
		t.Error("an unknown interval")
	}
	if _, err := billing.New(db, billing.Options{Plans: plans, FreePlan: "nope"}); err == nil {
		t.Error("an unknown free plan")
	}
	if billing.Team(4) != "team:4" || billing.User(9) != "user:9" {
		t.Error("owner keys")
	}
	b, err := billing.New(db, billing.Options{Plans: plans})
	if err != nil {
		t.Fatal(err)
	}
	j, _ := json.Marshal(b.Plans())
	if s := string(j); !strings.Contains(s, `"paid":false,"intervals":[]`) || !strings.Contains(s, `"paid":true,"intervals":["month","year"]`) || strings.Contains(s, "price_pro") {
		t.Errorf("the plans' JSON: %s", s)
	}
}
