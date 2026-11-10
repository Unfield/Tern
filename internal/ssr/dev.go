package ssr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Dev renders through the Vite dev server, so server rendering uses the
// same up-to-date modules as hot module replacement.
type Dev struct {
	URL    string // the Vite dev server, e.g. http://127.0.0.1:5174
	Client *http.Client
}

// Render implements Renderer.
func (d *Dev) Render(ctx context.Context, payload []byte, href string) (Result, error) {
	raw, err := d.post(ctx, "/__tern/render", struct {
		Payload string `json:"payload"`
		Href    string `json:"href"`
	}{string(payload), href})
	if err != nil {
		return Result{}, err
	}
	var res Result
	if err := json.Unmarshal(raw, &res); err != nil {
		return Result{}, fmt.Errorf("ssr: decoding the render result: %w", err)
	}
	return res, nil
}

// Email implements EmailRenderer.
func (d *Dev) Email(ctx context.Context, name string, props []byte) (EmailResult, []byte, error) {
	raw, err := d.post(ctx, "/__tern/email", struct {
		Name  string `json:"name"`
		Props string `json:"props"`
	}{name, string(props)})
	if err != nil {
		return EmailResult{}, nil, err
	}
	return decodeEmail(name, string(raw))
}

func (d *Dev) post(ctx context.Context, path string, v any) ([]byte, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.URL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := d.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ssr: contacting Vite: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ssr: %s", bytes.TrimSpace(raw))
	}
	return raw, nil
}
