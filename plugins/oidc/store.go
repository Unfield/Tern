package oidc

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/Unfield/Tern/sqldb"
)

// ErrNotFound is returned by a Store for missing or expired entries.
var ErrNotFound = errors.New("oidc: not found")

// AuthCode is an issued authorization code.
type AuthCode struct {
	ClientID      string   `json:"clientId"`
	RedirectURI   string   `json:"redirectUri"`
	Subject       string   `json:"subject"`
	Scopes        []string `json:"scopes"`
	Nonce         string   `json:"nonce,omitempty"`
	CodeChallenge string   `json:"codeChallenge,omitempty"`
	// Resource is the resource indicator the token is for (RFC 8707).
	Resource string    `json:"resource,omitempty"`
	AuthTime time.Time `json:"authTime"`
	Expires  time.Time `json:"expires"`
}

// RefreshToken is an issued refresh token. Tokens refreshed from one
// another share a Family; using an old one again revokes the family.
type RefreshToken struct {
	ClientID string    `json:"clientId"`
	Subject  string    `json:"subject"`
	Scopes   []string  `json:"scopes"`
	Family   string    `json:"family"`
	Resource string    `json:"resource,omitempty"`
	AuthTime time.Time `json:"authTime"`
	Expires  time.Time `json:"expires"`
	Used     bool      `json:"used"`
}

// Store keeps the provider's state. Codes and tokens are passed hashed, so
// a leaked store does not leak usable tokens. Use NewSQLStore in
// production; MemoryStore forgets everything on restart.
type Store interface {
	SaveCode(ctx context.Context, hash string, c AuthCode) error
	// TakeCode returns and deletes a code, so it can only be used once.
	TakeCode(ctx context.Context, hash string) (AuthCode, error)
	SaveRefresh(ctx context.Context, hash string, t RefreshToken) error
	GetRefresh(ctx context.Context, hash string) (RefreshToken, error)
	// UseRefresh marks a token used and reports whether it was unused.
	UseRefresh(ctx context.Context, hash string) (bool, error)
	RevokeRefresh(ctx context.Context, hash string) error
	RevokeFamily(ctx context.Context, family string) error
	// RevokeAccess records a revoked access token until it expires.
	RevokeAccess(ctx context.Context, jti string, expires time.Time) error
	AccessRevoked(ctx context.Context, jti string) (bool, error)
	SaveConsent(ctx context.Context, subject, clientID string, scopes []string) error
	// Consent returns the scopes a user approved for a client: nil if they
	// never did, and an empty, non-nil slice if they approved no scopes.
	Consent(ctx context.Context, subject, clientID string) ([]string, error)
	// SigningKey returns the stored signing key (PEM), or ErrNotFound.
	SigningKey(ctx context.Context) ([]byte, error)
	SaveSigningKey(ctx context.Context, pem []byte) error
}

// MemoryStore is a Store in memory, for development and tests.
type MemoryStore struct {
	mu       sync.Mutex
	codes    map[string]AuthCode
	refresh  map[string]RefreshToken
	revoked  map[string]time.Time
	consents map[string][]string
	key      []byte
}

// NewMemoryStore returns an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{codes: map[string]AuthCode{}, refresh: map[string]RefreshToken{}, revoked: map[string]time.Time{}, consents: map[string][]string{}}
}

// SaveCode implements Store.
func (s *MemoryStore) SaveCode(_ context.Context, h string, c AuthCode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.codes[h] = c
	return nil
}

// TakeCode implements Store.
func (s *MemoryStore) TakeCode(_ context.Context, h string) (AuthCode, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.codes[h]
	delete(s.codes, h)
	if !ok || time.Now().After(c.Expires) {
		return AuthCode{}, ErrNotFound
	}
	return c, nil
}

// SaveRefresh implements Store.
func (s *MemoryStore) SaveRefresh(_ context.Context, h string, t RefreshToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refresh[h] = t
	return nil
}

// GetRefresh implements Store.
func (s *MemoryStore) GetRefresh(_ context.Context, h string) (RefreshToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.refresh[h]
	if !ok || time.Now().After(t.Expires) {
		return RefreshToken{}, ErrNotFound
	}
	return t, nil
}

// UseRefresh implements Store.
func (s *MemoryStore) UseRefresh(_ context.Context, h string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.refresh[h]
	if !ok {
		return false, ErrNotFound
	}
	if t.Used {
		return false, nil
	}
	t.Used = true
	s.refresh[h] = t
	return true, nil
}

// RevokeRefresh implements Store.
func (s *MemoryStore) RevokeRefresh(_ context.Context, h string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.refresh, h)
	return nil
}

// RevokeFamily implements Store.
func (s *MemoryStore) RevokeFamily(_ context.Context, family string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for h, t := range s.refresh {
		if t.Family == family {
			delete(s.refresh, h)
		}
	}
	return nil
}

// RevokeAccess implements Store.
func (s *MemoryStore) RevokeAccess(_ context.Context, jti string, exp time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revoked[jti] = exp
	return nil
}

// AccessRevoked implements Store.
func (s *MemoryStore) AccessRevoked(_ context.Context, jti string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.revoked[jti]
	return ok, nil
}

// SaveConsent implements Store.
func (s *MemoryStore) SaveConsent(_ context.Context, sub, client string, scopes []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consents[sub+"\x00"+client] = slices.Clone(scopes)
	return nil
}

// Consent implements Store.
func (s *MemoryStore) Consent(_ context.Context, sub, client string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.consents[sub+"\x00"+client]), nil
}

// SigningKey implements Store.
func (s *MemoryStore) SigningKey(context.Context) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.key == nil {
		return nil, ErrNotFound
	}
	return s.key, nil
}

// SaveSigningKey implements Store.
func (s *MemoryStore) SaveSigningKey(_ context.Context, pem []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.key = pem
	return nil
}

// SQLStore keeps the provider's state in SQLite or PostgreSQL. It creates
// its tables (prefixed tern_oidc_) on first use.
type SQLStore struct {
	db   *sql.DB
	d    sqldb.Dialect
	once sync.Once
	err  error
}

// NewSQLStore returns a store in db.
func NewSQLStore(db *sql.DB) (*SQLStore, error) {
	d, err := sqldb.Detect(db)
	if err != nil {
		return nil, err
	}
	return &SQLStore{db: db, d: d}, nil
}

func (s *SQLStore) init(ctx context.Context) error {
	s.once.Do(func() {
		for _, q := range []string{
			`CREATE TABLE IF NOT EXISTS tern_oidc_codes (hash TEXT PRIMARY KEY, data TEXT NOT NULL, expires BIGINT NOT NULL)`,
			`CREATE TABLE IF NOT EXISTS tern_oidc_refresh (hash TEXT PRIMARY KEY, family TEXT NOT NULL, data TEXT NOT NULL, used INTEGER NOT NULL DEFAULT 0, expires BIGINT NOT NULL)`,
			`CREATE INDEX IF NOT EXISTS tern_oidc_refresh_family ON tern_oidc_refresh (family)`,
			`CREATE TABLE IF NOT EXISTS tern_oidc_revoked (jti TEXT PRIMARY KEY, expires BIGINT NOT NULL)`,
			`CREATE TABLE IF NOT EXISTS tern_oidc_consents (subject TEXT NOT NULL, client_id TEXT NOT NULL, scopes TEXT NOT NULL, PRIMARY KEY (subject, client_id))`,
			`CREATE TABLE IF NOT EXISTS tern_oidc_keys (id ` + s.d.AutoID() + `, pem TEXT NOT NULL)`,
		} {
			if _, err := s.db.ExecContext(ctx, q); err != nil {
				s.err = err
				return
			}
		}
	})
	return s.err
}

func (s *SQLStore) exec(ctx context.Context, q string, args ...any) error {
	if err := s.init(ctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, s.d.Rebind(q), args...)
	return err
}

// SaveCode implements Store.
func (s *SQLStore) SaveCode(ctx context.Context, h string, c AuthCode) error {
	b, _ := json.Marshal(c)
	// Codes live a minute; clearing old ones here keeps the table small.
	if err := s.exec(ctx, `DELETE FROM tern_oidc_codes WHERE expires < ?`, time.Now().UnixMilli()); err != nil {
		return err
	}
	return s.exec(ctx, `INSERT INTO tern_oidc_codes (hash, data, expires) VALUES (?, ?, ?)`, h, string(b), c.Expires.UnixMilli())
}

// TakeCode implements Store.
func (s *SQLStore) TakeCode(ctx context.Context, h string) (AuthCode, error) {
	if err := s.init(ctx); err != nil {
		return AuthCode{}, err
	}
	var data string
	err := s.db.QueryRowContext(ctx, s.d.Rebind(`DELETE FROM tern_oidc_codes WHERE hash = ? RETURNING data`), h).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return AuthCode{}, ErrNotFound
	}
	if err != nil {
		return AuthCode{}, err
	}
	var c AuthCode
	if err := json.Unmarshal([]byte(data), &c); err != nil || time.Now().After(c.Expires) {
		return AuthCode{}, ErrNotFound
	}
	return c, nil
}

// SaveRefresh implements Store.
func (s *SQLStore) SaveRefresh(ctx context.Context, h string, t RefreshToken) error {
	b, _ := json.Marshal(t)
	if err := s.exec(ctx, `DELETE FROM tern_oidc_refresh WHERE expires < ?`, time.Now().UnixMilli()); err != nil {
		return err
	}
	return s.exec(ctx, `INSERT INTO tern_oidc_refresh (hash, family, data, used, expires) VALUES (?, ?, ?, 0, ?)`, h, t.Family, string(b), t.Expires.UnixMilli())
}

// GetRefresh implements Store.
func (s *SQLStore) GetRefresh(ctx context.Context, h string) (RefreshToken, error) {
	if err := s.init(ctx); err != nil {
		return RefreshToken{}, err
	}
	var data string
	var used bool
	err := s.db.QueryRowContext(ctx, s.d.Rebind(`SELECT data, used FROM tern_oidc_refresh WHERE hash = ?`), h).Scan(&data, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return RefreshToken{}, ErrNotFound
	}
	if err != nil {
		return RefreshToken{}, err
	}
	var t RefreshToken
	if err := json.Unmarshal([]byte(data), &t); err != nil || time.Now().After(t.Expires) {
		return RefreshToken{}, ErrNotFound
	}
	t.Used = used
	return t, nil
}

// UseRefresh implements Store.
func (s *SQLStore) UseRefresh(ctx context.Context, h string) (bool, error) {
	if err := s.init(ctx); err != nil {
		return false, err
	}
	res, err := s.db.ExecContext(ctx, s.d.Rebind(`UPDATE tern_oidc_refresh SET used = 1 WHERE hash = ? AND used = 0`), h)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// RevokeRefresh implements Store.
func (s *SQLStore) RevokeRefresh(ctx context.Context, h string) error {
	return s.exec(ctx, `DELETE FROM tern_oidc_refresh WHERE hash = ?`, h)
}

// RevokeFamily implements Store.
func (s *SQLStore) RevokeFamily(ctx context.Context, family string) error {
	return s.exec(ctx, `DELETE FROM tern_oidc_refresh WHERE family = ?`, family)
}

// RevokeAccess implements Store.
func (s *SQLStore) RevokeAccess(ctx context.Context, jti string, exp time.Time) error {
	if err := s.exec(ctx, `DELETE FROM tern_oidc_revoked WHERE expires < ?`, time.Now().UnixMilli()); err != nil {
		return err
	}
	return s.exec(ctx, `INSERT INTO tern_oidc_revoked (jti, expires) VALUES (?, ?) ON CONFLICT DO NOTHING`, jti, exp.UnixMilli())
}

// AccessRevoked implements Store.
func (s *SQLStore) AccessRevoked(ctx context.Context, jti string) (bool, error) {
	if err := s.init(ctx); err != nil {
		return false, err
	}
	var n int
	err := s.db.QueryRowContext(ctx, s.d.Rebind(`SELECT COUNT(*) FROM tern_oidc_revoked WHERE jti = ?`), jti).Scan(&n)
	return n > 0, err
}

// SaveConsent implements Store.
func (s *SQLStore) SaveConsent(ctx context.Context, sub, client string, scopes []string) error {
	b, _ := json.Marshal(scopes)
	return s.exec(ctx, `INSERT INTO tern_oidc_consents (subject, client_id, scopes) VALUES (?, ?, ?)
		ON CONFLICT (subject, client_id) DO UPDATE SET scopes = excluded.scopes`, sub, client, string(b))
}

// Consent implements Store.
func (s *SQLStore) Consent(ctx context.Context, sub, client string) ([]string, error) {
	if err := s.init(ctx); err != nil {
		return nil, err
	}
	var data string
	err := s.db.QueryRowContext(ctx, s.d.Rebind(`SELECT scopes FROM tern_oidc_consents WHERE subject = ? AND client_id = ?`), sub, client).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	var scopes []string
	_ = json.Unmarshal([]byte(data), &scopes)
	return scopes, err
}

// SigningKey implements Store.
func (s *SQLStore) SigningKey(ctx context.Context) ([]byte, error) {
	if err := s.init(ctx); err != nil {
		return nil, err
	}
	var pem string
	err := s.db.QueryRowContext(ctx, `SELECT pem FROM tern_oidc_keys ORDER BY id DESC LIMIT 1`).Scan(&pem)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return []byte(pem), err
}

// SaveSigningKey implements Store.
func (s *SQLStore) SaveSigningKey(ctx context.Context, pem []byte) error {
	return s.exec(ctx, `INSERT INTO tern_oidc_keys (pem) VALUES (?)`, string(pem))
}
