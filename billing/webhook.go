package billing

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
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Unfield/Tern/jobs"
	"github.com/Unfield/Tern/kit"
)

// signatureTolerance is how old a webhook's signed timestamp may be: an
// older one is a replay.
const signatureTolerance = 5 * time.Minute

// ErrSignature is a webhook whose signature doesn't check out.
var ErrSignature = errors.New("billing: the webhook's signature is invalid")

// VerifySignature checks a Stripe-Signature header against the payload
// and the endpoint's secret, as Stripe's libraries do.
func VerifySignature(payload []byte, header, secret string, now time.Time) error {
	var ts int64
	var sigs []string
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			ts, _ = strconv.ParseInt(v, 10, 64)
		case "v1":
			sigs = append(sigs, v)
		}
	}
	if ts == 0 || len(sigs) == 0 || secret == "" {
		return ErrSignature
	}
	if d := now.Sub(time.Unix(ts, 0)); d > signatureTolerance || d < -signatureTolerance {
		return ErrSignature
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	mac.Write([]byte("."))
	mac.Write(payload)
	want := mac.Sum(nil)
	for _, s := range sigs {
		got, err := hex.DecodeString(s)
		if err == nil && hmac.Equal(got, want) {
			return nil
		}
	}
	return ErrSignature
}

// event is what billing reads of a Stripe event.
type event struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data struct {
		Object struct {
			Object       string `json:"object"`
			ID           string `json:"id"`
			Subscription any    `json:"subscription"` // an ID, or expanded
			Parent       struct {
				SubscriptionDetails struct {
					Subscription string `json:"subscription"`
				} `json:"subscription_details"`
			} `json:"parent"`
		} `json:"object"`
	} `json:"data"`
}

// subscriptionID is the subscription an event is about, "" for none.
func (e *event) subscriptionID() string {
	o := e.Data.Object
	if o.Object == "subscription" {
		return o.ID
	}
	switch v := o.Subscription.(type) {
	case string:
		return v
	case map[string]any:
		id, _ := v["id"].(string)
		return id
	}
	// Invoices since the 2025-03-31 API name it under parent.
	return o.Parent.SubscriptionDetails.Subscription
}

type syncJob struct {
	Subscription string `json:"subscription"`
}

// webhook takes Stripe's events: it checks the signature, records the
// event so a repeat is ignored, and syncs the subscription it's about, in
// a job when there's a queue.
func (b *Billing) webhook(c *kit.Ctx) error {
	payload, err := io.ReadAll(http.MaxBytesReader(nil, c.Request.Body, 1<<20))
	if err != nil {
		return kit.Error(http.StatusBadRequest, "unreadable body")
	}
	if err := VerifySignature(payload, c.Request.Header.Get("Stripe-Signature"), b.opts.WebhookSecret, time.Now()); err != nil {
		return kit.Error(http.StatusBadRequest, err.Error())
	}
	var e event
	if err := json.Unmarshal(payload, &e); err != nil || e.ID == "" {
		return kit.Error(http.StatusBadRequest, "not a Stripe event")
	}
	ctx := c.Request.Context()
	sub := e.subscriptionID()
	if sub == "" {
		return c.JSON(http.StatusOK, map[string]bool{"received": true})
	}
	// Recorded with the job in one transaction: a crash between the two
	// leaves neither, and Stripe sends the event again.
	tx, err := b.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, b.d.Rebind(`INSERT INTO tern_billing_events (id, type, received_at) VALUES (?, ?, ?) ON CONFLICT (id) DO NOTHING`), e.ID, e.Type, time.Now().UnixMilli())
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return c.JSON(http.StatusOK, map[string]bool{"received": true, "duplicate": true})
	}
	if b.sync != nil {
		// One job per event, not one per subscription: a job already
		// running may have fetched the subscription before this change.
		// Each job fetches it again, so repeats are harmless.
		if _, err := b.sync.Enqueue(ctx, syncJob{Subscription: sub}, jobs.InTx(tx)); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		return c.JSON(http.StatusOK, map[string]bool{"received": true})
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if err := b.SyncSubscription(ctx, sub); err != nil {
		// Forget the event, so Stripe's retry is processed.
		_, _ = b.exec(context.WithoutCancel(ctx), `DELETE FROM tern_billing_events WHERE id = ?`, e.ID)
		return err
	}
	return c.JSON(http.StatusOK, map[string]bool{"received": true})
}

func (b *Billing) runSync(ctx context.Context, j syncJob) error {
	return b.SyncSubscription(ctx, j.Subscription)
}

// SyncSubscription fetches a subscription from Stripe and stores the
// app's copy: what the webhooks do. Fetching it, instead of trusting the
// event, makes events that arrive late or out of order harmless.
func (b *Billing) SyncSubscription(ctx context.Context, id string) error {
	var s stripeSubscription
	if err := b.api.get(ctx, "/v1/subscriptions/"+id, &s); err != nil {
		return err
	}
	owner := s.Metadata["owner"]
	if owner == "" {
		err := b.queryRow(ctx, `SELECT owner FROM tern_billing_customers WHERE customer_id = ?`, s.Customer).Scan(&owner)
		if errors.Is(err, sql.ErrNoRows) {
			return nil // not one of the app's: made in Stripe's dashboard for someone else
		}
		if err != nil {
			return err
		}
	}
	if len(s.Items.Data) == 0 {
		return fmt.Errorf("billing: subscription %s has no items", id)
	}
	item := s.Items.Data[0]
	ref, ok := b.prices[item.Price.ID]
	if !ok {
		return fmt.Errorf("billing: subscription %s has price %s, which no plan has", id, item.Price.ID)
	}
	period := item.CurrentPeriodEnd
	if period == 0 {
		period = s.CurrentPeriodEnd
	}
	var trial any
	if s.TrialEnd > 0 {
		trial = s.TrialEnd
	}
	cancel := 0
	if s.CancelAtPeriodEnd {
		cancel = 1
	}
	next := Subscription{Owner: owner, Plan: ref.plan, Status: Status(s.Status), Interval: ref.interval, Seats: max(item.Quantity, 1), SubscriptionID: s.ID, CustomerID: s.Customer}
	// One subscription per owner: a canceled one doesn't replace an active
	// one, as when a customer resubscribes before the old one ends.
	cur, err := b.Subscription(ctx, owner)
	if err != nil {
		return err
	}
	if cur != nil && cur.SubscriptionID != s.ID && cur.Active() && !next.Active() {
		return nil
	}
	now := time.Now().UnixMilli()
	if _, err := b.exec(ctx, `INSERT INTO tern_billing_subscriptions (owner, subscription_id, customer_id, plan, status, billing_interval, seats, current_period_end, cancel_at_period_end, trial_end, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (owner) DO UPDATE SET subscription_id = excluded.subscription_id, customer_id = excluded.customer_id, plan = excluded.plan, status = excluded.status,
			billing_interval = excluded.billing_interval, seats = excluded.seats, current_period_end = excluded.current_period_end,
			cancel_at_period_end = excluded.cancel_at_period_end, trial_end = excluded.trial_end, updated_at = excluded.updated_at`,
		owner, s.ID, s.Customer, ref.plan, s.Status, string(ref.interval), next.Seats, period, cancel, trial, now); err != nil {
		return err
	}
	if _, err := b.exec(ctx, `INSERT INTO tern_billing_customers (owner, customer_id, created_at) VALUES (?, ?, ?) ON CONFLICT (owner) DO NOTHING`, owner, s.Customer, now); err != nil {
		return err
	}
	if b.opts.OnChange != nil {
		if stored, err := b.Subscription(ctx, owner); err == nil && stored != nil {
			b.opts.OnChange(ctx, *stored)
		}
	}
	return nil
}
