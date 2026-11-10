package oidc_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/Unfield/Tern/internal/dbtest"
	"github.com/Unfield/Tern/plugins/oidc"
)

func TestStores(t *testing.T) {
	t.Run("memory", func(t *testing.T) { storeContract(t, oidc.NewMemoryStore()) })
	dbtest.Each(t, func(t *testing.T, db *sql.DB) {
		s, err := oidc.NewSQLStore(db)
		if err != nil {
			t.Fatal(err)
		}
		storeContract(t, s)
	})
}

func storeContract(t *testing.T, s oidc.Store) {
	ctx := context.Background()
	soon := time.Now().Add(time.Minute)

	if err := s.SaveCode(ctx, "h1", oidc.AuthCode{ClientID: "c", Scopes: []string{"openid"}, Expires: soon}); err != nil {
		t.Fatal(err)
	}
	if c, err := s.TakeCode(ctx, "h1"); err != nil || c.ClientID != "c" || c.Scopes[0] != "openid" {
		t.Fatalf("TakeCode = %+v, %v", c, err)
	}
	if _, err := s.TakeCode(ctx, "h1"); !errors.Is(err, oidc.ErrNotFound) {
		t.Errorf("a code can be taken twice: %v", err)
	}
	_ = s.SaveCode(ctx, "old", oidc.AuthCode{Expires: time.Now().Add(-time.Second)})
	if _, err := s.TakeCode(ctx, "old"); !errors.Is(err, oidc.ErrNotFound) {
		t.Errorf("an expired code was returned: %v", err)
	}

	for _, h := range []string{"r1", "r2"} {
		if err := s.SaveRefresh(ctx, h, oidc.RefreshToken{ClientID: "c", Family: "f", Expires: soon}); err != nil {
			t.Fatal(err)
		}
	}
	if ok, err := s.UseRefresh(ctx, "r1"); !ok || err != nil {
		t.Errorf("first use = %v, %v", ok, err)
	}
	if ok, _ := s.UseRefresh(ctx, "r1"); ok {
		t.Error("a refresh token was used twice")
	}
	if r, err := s.GetRefresh(ctx, "r1"); err != nil || !r.Used {
		t.Errorf("GetRefresh = %+v, %v", r, err)
	}
	if err := s.RevokeFamily(ctx, "f"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetRefresh(ctx, "r2"); !errors.Is(err, oidc.ErrNotFound) {
		t.Errorf("the family was not revoked: %v", err)
	}

	if err := s.RevokeAccess(ctx, "jti", soon); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.AccessRevoked(ctx, "jti"); !ok {
		t.Error("revoked access token not reported")
	}
	if ok, _ := s.AccessRevoked(ctx, "other"); ok {
		t.Error("an unrevoked token was reported revoked")
	}

	if got, _ := s.Consent(ctx, "alice", "c"); len(got) != 0 {
		t.Errorf("consent before saving = %v", got)
	}
	_ = s.SaveConsent(ctx, "alice", "c", []string{"openid"})
	_ = s.SaveConsent(ctx, "alice", "c", []string{"openid", "email"})
	if got, _ := s.Consent(ctx, "alice", "c"); len(got) != 2 {
		t.Errorf("consent = %v", got)
	}

	if _, err := s.SigningKey(ctx); !errors.Is(err, oidc.ErrNotFound) {
		t.Errorf("SigningKey before saving: %v", err)
	}
	if err := s.SaveSigningKey(ctx, []byte("pem")); err != nil {
		t.Fatal(err)
	}
	if k, err := s.SigningKey(ctx); err != nil || string(k) != "pem" {
		t.Errorf("SigningKey = %q, %v", k, err)
	}
}
