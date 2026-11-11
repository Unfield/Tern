// Package billing charges for an app with Stripe: plans defined in Go,
// Stripe Checkout to subscribe, the customer portal to change or cancel,
// trials and seats, and limits per plan checked in loads and actions.
//
// Stripe's webhooks keep the app's copy of each subscription current.
// They are verified, recorded once, and processed as jobs, which fetch
// the subscription from Stripe: a slow, repeated or out-of-order event
// is safe.
//
//	b, err := billing.New(db, billing.Options{
//		SecretKey:     os.Getenv("STRIPE_SECRET_KEY"),
//		WebhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
//		Jobs:          queue,
//		Plans: []billing.Plan{
//			{ID: "free", Name: "Free", Limits: map[string]int{"projects": 3}},
//			{ID: "pro", Name: "Pro", Prices: map[billing.Interval]string{billing.Monthly: "price_…"}, TrialDays: 14},
//		},
//	})
//	err = b.Migrate(ctx)
//	app, err := ternApp(kit.WithPlugins(b))
//
// Who pays is an owner: a string the app chooses, such as billing.Team(id)
// for a team or billing.User(id) for a user.
package billing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Unfield/Tern/jobs"
	"github.com/Unfield/Tern/kit"
	"github.com/Unfield/Tern/sqldb"
)

// Interval is how often a price is charged.
type Interval string

// The intervals.
const (
	Monthly Interval = "month"
	Yearly  Interval = "year"
)

// Plan is what a customer can subscribe to.
type Plan struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	// Prices are the plan's Stripe price IDs, by interval. A plan without
	// prices is free: everyone without a subscription is on it.
	Prices map[Interval]string `json:"-"`
	// Amounts are what the pricing page shows, in the currency's smallest
	// unit (cents), by interval. Stripe charges what its prices say: keep
	// them the same.
	Amounts  map[Interval]int64 `json:"amounts,omitempty"`
	Currency string             `json:"currency,omitempty"` // "usd", "eur"
	// Limits cap what the plan allows: {"projects": 10, "members": 5}. A
	// limit the plan doesn't list is unlimited.
	Limits map[string]int `json:"limits,omitempty"`
	// Features are what the plan includes, for the pricing page and Has.
	Features []string `json:"features,omitempty"`
	// TrialDays is a free trial before the first charge.
	TrialDays int `json:"trialDays,omitempty"`
	// PerSeat charges per member: the subscription's quantity is the seats.
	PerSeat bool `json:"perSeat,omitempty"`
	// Highlight marks the plan to recommend on the pricing page.
	Highlight bool `json:"highlight,omitempty"`

	// Paid and Intervals are for the pricing page, which doesn't see the
	// prices: whether the plan costs money, and the intervals it's paid
	// by. New sets them.
	Paid      bool       `json:"paid"`
	Intervals []Interval `json:"intervals"`
}

// Free reports whether the plan costs nothing.
func (p Plan) Free() bool { return len(p.Prices) == 0 }

// Status is a subscription's state, as Stripe names it.
type Status string

// The statuses.
const (
	Active            Status = "active"
	Trialing          Status = "trialing"
	PastDue           Status = "past_due"
	Canceled          Status = "canceled"
	Unpaid            Status = "unpaid"
	Incomplete        Status = "incomplete"
	IncompleteExpired Status = "incomplete_expired"
	Paused            Status = "paused"
)

// Subscription is the app's copy of an owner's Stripe subscription.
type Subscription struct {
	Owner    string   `json:"owner"`
	Plan     string   `json:"plan"`
	Status   Status   `json:"status"`
	Interval Interval `json:"interval"`
	Seats    int      `json:"seats"`
	// CurrentPeriodEnd is when it renews, or ends if CancelAtPeriodEnd.
	CurrentPeriodEnd  time.Time  `json:"currentPeriodEnd"`
	CancelAtPeriodEnd bool       `json:"cancelAtPeriodEnd"`
	TrialEnd          *time.Time `json:"trialEnd"`
	CustomerID        string     `json:"-"`
	SubscriptionID    string     `json:"-"`
	UpdatedAt         time.Time  `json:"updatedAt"`
}

// Active reports whether the subscription's plan applies: it's paid, in
// its trial, or a payment failed and Stripe is still retrying.
func (s *Subscription) Active() bool {
	return s != nil && (s.Status == Active || s.Status == Trialing || s.Status == PastDue)
}

// Team is the owner key of a team.
func Team(id int64) string { return "team:" + strconv.FormatInt(id, 10) }

// User is the owner key of a user.
func User(id int64) string { return "user:" + strconv.FormatInt(id, 10) }

// Options configure billing.
type Options struct {
	// SecretKey is Stripe's secret API key, sk_test_… or sk_live_….
	SecretKey string
	// WebhookSecret signs Stripe's webhooks to the app, whsec_…, from the
	// webhook endpoint's settings or `stripe listen`.
	WebhookSecret string
	// Plans are the plans, in the order the pricing page shows them.
	Plans []Plan
	// FreePlan is the plan of owners without an active subscription. The
	// default is the first plan without prices.
	FreePlan string
	// Jobs processes webhooks in the background, with retries. Without
	// it, they're processed in the webhook's request.
	Jobs *jobs.Queue
	// WebhookPath is where Stripe posts events. The default is
	// /_tern/billing/webhook.
	WebhookPath string
	// OnChange is called after an owner's subscription changes, from the
	// webhook's job: to email a receipt, or record it in the audit log.
	OnChange func(ctx context.Context, s Subscription)

	// APIBase is Stripe's API address, for tests. The default is
	// https://api.stripe.com.
	APIBase string
	// HTTPClient sends the API requests. The default has a 30s timeout.
	HTTPClient *http.Client
}

// Billing is the billing plugin. Create it with New.
type Billing struct {
	db     *sql.DB
	d      sqldb.Dialect
	opts   Options
	plans  map[string]Plan
	prices map[string]priceRef // Stripe price ID → plan and interval
	free   Plan
	api    *stripeAPI
	sync   *jobs.Kind[syncJob]
}

type priceRef struct {
	plan     string
	interval Interval
}

// The errors.
var (
	ErrUnknownPlan = errors.New("billing: unknown plan or interval")
	ErrNoCustomer  = errors.New("billing: this owner has never subscribed")
	ErrNotSet      = errors.New("billing: Options.SecretKey is not set")
)

// New returns the billing plugin.
func New(db *sql.DB, opts Options) (*Billing, error) {
	d, err := sqldb.Detect(db)
	if err != nil {
		return nil, fmt.Errorf("billing: %w", err)
	}
	if opts.WebhookPath == "" {
		opts.WebhookPath = "/_tern/billing/webhook"
	}
	b := &Billing{db: db, d: d, opts: opts, plans: map[string]Plan{}, prices: map[string]priceRef{}}
	opts.Plans = slices.Clone(opts.Plans)
	for i := range opts.Plans {
		p := &opts.Plans[i]
		p.Paid, p.Intervals = !p.Free(), []Interval{}
		for _, iv := range []Interval{Monthly, Yearly} {
			if _, ok := p.Prices[iv]; ok {
				p.Intervals = append(p.Intervals, iv)
			}
		}
	}
	b.opts.Plans = opts.Plans
	for _, p := range opts.Plans {
		if p.ID == "" {
			return nil, errors.New("billing: a plan needs an ID")
		}
		if _, dup := b.plans[p.ID]; dup {
			return nil, fmt.Errorf("billing: plan %q defined twice", p.ID)
		}
		b.plans[p.ID] = p
		for iv, price := range p.Prices {
			if iv != Monthly && iv != Yearly {
				return nil, fmt.Errorf("billing: plan %q: interval %q isn't %q or %q", p.ID, iv, Monthly, Yearly)
			}
			b.prices[price] = priceRef{plan: p.ID, interval: iv}
		}
	}
	switch {
	case opts.FreePlan != "":
		p, ok := b.plans[opts.FreePlan]
		if !ok {
			return nil, fmt.Errorf("billing: FreePlan %q isn't one of the plans", opts.FreePlan)
		}
		b.free = p
	default:
		for _, p := range opts.Plans {
			if p.Free() {
				b.free = p
				break
			}
		}
	}
	base := strings.TrimSuffix(opts.APIBase, "/")
	if base == "" {
		base = "https://api.stripe.com"
	}
	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	b.api = &stripeAPI{base: base, key: opts.SecretKey, http: hc}
	if opts.Jobs != nil {
		b.sync = jobs.Handle(opts.Jobs, "tern.billing.sync", b.runSync, jobs.MaxAttempts(8))
	}
	return b, nil
}

// Migrate creates the tables, if they don't exist yet.
func (b *Billing) Migrate(ctx context.Context) error {
	for _, s := range []string{
		`CREATE TABLE IF NOT EXISTS tern_billing_customers (
			owner TEXT PRIMARY KEY,
			customer_id TEXT NOT NULL UNIQUE,
			created_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS tern_billing_subscriptions (
			owner TEXT PRIMARY KEY,
			subscription_id TEXT NOT NULL UNIQUE,
			customer_id TEXT NOT NULL,
			plan TEXT NOT NULL,
			status TEXT NOT NULL,
			billing_interval TEXT NOT NULL,
			seats INTEGER NOT NULL,
			current_period_end BIGINT NOT NULL,
			cancel_at_period_end INTEGER NOT NULL,
			trial_end BIGINT,
			updated_at BIGINT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS tern_billing_events (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			received_at BIGINT NOT NULL
		)`,
	} {
		if _, err := b.db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("billing: creating tables: %w", err)
		}
	}
	return nil
}

func (b *Billing) exec(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return b.db.ExecContext(ctx, b.d.Rebind(q), args...)
}

func (b *Billing) queryRow(ctx context.Context, q string, args ...any) *sql.Row {
	return b.db.QueryRowContext(ctx, b.d.Rebind(q), args...)
}

// Name implements kit.Plugin.
func (*Billing) Name() string { return "billing" }

// Setup implements kit.Plugin: it serves the webhook endpoint.
func (b *Billing) Setup(s *kit.Setup) error {
	s.Endpoint("POST "+b.opts.WebhookPath, b.webhook)
	// Stripe signs its posts; they carry no cookies to protect.
	s.AllowCrossOrigin("POST " + b.opts.WebhookPath)
	return nil
}

// Plans returns the plans, in order.
func (b *Billing) Plans() []Plan { return b.opts.Plans }

// Plan returns a plan by its ID.
func (b *Billing) Plan(id string) (Plan, bool) {
	p, ok := b.plans[id]
	return p, ok
}

// Subscription returns the owner's subscription, active or not, or nil if
// they never had one.
func (b *Billing) Subscription(ctx context.Context, owner string) (*Subscription, error) {
	var s Subscription
	var status, interval string
	var period, updated int64
	var cancel int
	var trial sql.NullInt64
	err := b.queryRow(ctx, `SELECT owner, subscription_id, customer_id, plan, status, billing_interval, seats, current_period_end, cancel_at_period_end, trial_end, updated_at
		FROM tern_billing_subscriptions WHERE owner = ?`, owner).
		Scan(&s.Owner, &s.SubscriptionID, &s.CustomerID, &s.Plan, &status, &interval, &s.Seats, &period, &cancel, &trial, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.Status, s.Interval = Status(status), Interval(interval)
	s.CurrentPeriodEnd = time.Unix(period, 0).UTC()
	s.CancelAtPeriodEnd = cancel == 1
	if trial.Valid {
		t := time.Unix(trial.Int64, 0).UTC()
		s.TrialEnd = &t
	}
	s.UpdatedAt = time.UnixMilli(updated).UTC()
	return &s, nil
}

// Current is the plan that applies to the owner: their subscription's
// while it's active, else the free plan. The subscription is nil if they
// never had one.
func (b *Billing) Current(ctx context.Context, owner string) (Plan, *Subscription, error) {
	s, err := b.Subscription(ctx, owner)
	if err != nil {
		return Plan{}, nil, err
	}
	if s.Active() {
		if p, ok := b.plans[s.Plan]; ok {
			return p, s, nil
		}
	}
	return b.free, s, nil
}

// CheckoutOptions describe a Checkout for Checkout.
type CheckoutOptions struct {
	Owner    string
	Plan     string
	Interval Interval
	// Email fills in the customer's email the first time.
	Email string
	// Seats is the quantity of a per-seat plan; at least 1.
	Seats int
	// SuccessURL and CancelURL are where Stripe sends the customer back:
	// absolute, or paths on the app's own address.
	SuccessURL, CancelURL string
}

// Checkout starts Stripe Checkout to subscribe the owner to a plan, and
// returns its address to redirect to. The plan's trial applies to owners
// who haven't subscribed before.
func (b *Billing) Checkout(c *kit.Ctx, o CheckoutOptions) (string, error) {
	if b.opts.SecretKey == "" {
		return "", ErrNotSet
	}
	p, ok := b.plans[o.Plan]
	if !ok {
		return "", ErrUnknownPlan
	}
	price, ok := p.Prices[o.Interval]
	if !ok {
		return "", ErrUnknownPlan
	}
	ctx := c.Request.Context()
	customer, existed, err := b.customer(ctx, o.Owner, o.Email)
	if err != nil {
		return "", err
	}
	seats := 1
	if p.PerSeat && o.Seats > 1 {
		seats = o.Seats
	}
	v := map[string]string{
		"mode":                               "subscription",
		"customer":                           customer,
		"client_reference_id":                o.Owner,
		"line_items[0][price]":               price,
		"line_items[0][quantity]":            strconv.Itoa(seats),
		"subscription_data[metadata][owner]": o.Owner,
		"success_url":                        absolute(c, o.SuccessURL),
		"cancel_url":                         absolute(c, o.CancelURL),
		"allow_promotion_codes":              "true",
	}
	if p.TrialDays > 0 && !existed {
		v["subscription_data[trial_period_days]"] = strconv.Itoa(p.TrialDays)
	}
	var session struct {
		URL string `json:"url"`
	}
	if err := b.api.post(ctx, "/v1/checkout/sessions", v, &session, ""); err != nil {
		return "", err
	}
	return session.URL, nil
}

// Portal opens Stripe's customer portal for the owner, where they change
// their plan or payment method, see invoices, or cancel, and returns its
// address to redirect to. returnURL is where its back link goes.
func (b *Billing) Portal(c *kit.Ctx, owner, returnURL string) (string, error) {
	if b.opts.SecretKey == "" {
		return "", ErrNotSet
	}
	ctx := c.Request.Context()
	var customer string
	err := b.queryRow(ctx, `SELECT customer_id FROM tern_billing_customers WHERE owner = ?`, owner).Scan(&customer)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNoCustomer
	}
	if err != nil {
		return "", err
	}
	var session struct {
		URL string `json:"url"`
	}
	if err := b.api.post(ctx, "/v1/billing_portal/sessions", map[string]string{"customer": customer, "return_url": absolute(c, returnURL)}, &session, ""); err != nil {
		return "", err
	}
	return session.URL, nil
}

// customer returns the owner's Stripe customer, creating it the first
// time; existed is whether it was there before.
func (b *Billing) customer(ctx context.Context, owner, email string) (id string, existed bool, err error) {
	err = b.queryRow(ctx, `SELECT customer_id FROM tern_billing_customers WHERE owner = ?`, owner).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", false, err
	}
	v := map[string]string{"metadata[owner]": owner}
	if email != "" {
		v["email"] = email
	}
	var cust struct {
		ID string `json:"id"`
	}
	// The idempotency key makes a retried request, or two at once, one
	// customer.
	if err := b.api.post(ctx, "/v1/customers", v, &cust, "customer:"+owner); err != nil {
		return "", false, err
	}
	if _, err := b.exec(ctx, `INSERT INTO tern_billing_customers (owner, customer_id, created_at) VALUES (?, ?, ?) ON CONFLICT (owner) DO NOTHING`, owner, cust.ID, time.Now().UnixMilli()); err != nil {
		return "", false, err
	}
	err = b.queryRow(ctx, `SELECT customer_id FROM tern_billing_customers WHERE owner = ?`, owner).Scan(&id)
	return id, false, err
}

// SetSeats changes the quantity of the owner's per-seat subscription, as
// members join or leave. Stripe prorates the change. It does nothing for
// owners without an active per-seat subscription.
func (b *Billing) SetSeats(ctx context.Context, owner string, seats int) error {
	s, err := b.Subscription(ctx, owner)
	if err != nil || !s.Active() {
		return err
	}
	p, ok := b.plans[s.Plan]
	if !ok || !p.PerSeat || seats < 1 || seats == s.Seats {
		return nil
	}
	var sub stripeSubscription
	if err := b.api.get(ctx, "/v1/subscriptions/"+s.SubscriptionID, &sub); err != nil {
		return err
	}
	if len(sub.Items.Data) == 0 {
		return fmt.Errorf("billing: subscription %s has no items", s.SubscriptionID)
	}
	if err := b.api.post(ctx, "/v1/subscription_items/"+sub.Items.Data[0].ID, map[string]string{"quantity": strconv.Itoa(seats)}, nil, ""); err != nil {
		return err
	}
	return b.SyncSubscription(ctx, s.SubscriptionID)
}

// absolute makes a path into an address on the app's own host.
func absolute(c *kit.Ctx, u string) string {
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		return u
	}
	scheme := "https"
	if c.Request.TLS == nil && !strings.EqualFold(c.Request.Header.Get("X-Forwarded-Proto"), "https") {
		if h := c.Request.Host; strings.HasPrefix(h, "localhost") || strings.HasPrefix(h, "127.0.0.1") {
			scheme = "http"
		}
	}
	if u == "" {
		u = "/"
	}
	return scheme + "://" + c.Request.Host + c.BasePath() + u
}
