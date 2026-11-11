package billing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// stripeAPI speaks Stripe's REST API: form-encoded requests with the
// secret key, JSON answers. Only the calls billing makes.
type stripeAPI struct {
	base string
	key  string
	http *http.Client
}

// StripeError is an error Stripe answered with.
type StripeError struct {
	Status  int
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *StripeError) Error() string {
	return fmt.Sprintf("billing: Stripe answered %d: %s", e.Status, e.Message)
}

func (s *stripeAPI) post(ctx context.Context, path string, params map[string]string, out any, idempotencyKey string) error {
	form := url.Values{}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		form.Set(k, params[k])
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.base+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	return s.do(req, out)
}

func (s *stripeAPI) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.base+path, nil)
	if err != nil {
		return err
	}
	return s.do(req, out)
}

func (s *stripeAPI) do(req *http.Request, out any) error {
	if s.key == "" {
		return ErrNotSet
	}
	req.Header.Set("Authorization", "Bearer "+s.key)
	req.Header.Set("Stripe-Version", stripeVersion)
	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("billing: calling Stripe: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("billing: reading Stripe's answer: %w", err)
	}
	if resp.StatusCode >= 300 {
		var e struct {
			Error StripeError `json:"error"`
		}
		_ = json.Unmarshal(body, &e)
		e.Error.Status = resp.StatusCode
		if e.Error.Message == "" {
			e.Error.Message = http.StatusText(resp.StatusCode)
		}
		return &e.Error
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(body, out)
}

// stripeVersion pins the API version billing is written against, so a
// change to the account's default doesn't change the answers.
const stripeVersion = "2025-03-31.basil"

// stripeSubscription is what billing reads of a Stripe subscription.
type stripeSubscription struct {
	ID       string            `json:"id"`
	Customer string            `json:"customer"`
	Status   string            `json:"status"`
	Metadata map[string]string `json:"metadata"`
	// Since the 2025-03-31 API, the period is on each item; it was here
	// before.
	CurrentPeriodEnd  int64 `json:"current_period_end"`
	CancelAtPeriodEnd bool  `json:"cancel_at_period_end"`
	TrialEnd          int64 `json:"trial_end"`
	Items             struct {
		Data []struct {
			ID               string `json:"id"`
			Quantity         int    `json:"quantity"`
			CurrentPeriodEnd int64  `json:"current_period_end"`
			Price            struct {
				ID        string `json:"id"`
				Recurring struct {
					Interval string `json:"interval"`
				} `json:"recurring"`
			} `json:"price"`
		} `json:"data"`
	} `json:"items"`
}
