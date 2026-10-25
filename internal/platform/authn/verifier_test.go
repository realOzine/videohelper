package authn

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/config"
)

func TestVerifyJWKSAndRotation(t *testing.T) {
	first := rsaKey(t)
	second := rsaKey(t)
	var keys atomic.Value
	keys.Store(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{publicKey(first, "first")}})
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(keys.Load())
	}))
	defer server.Close()
	cfg := config.AuthConfig{Issuer: server.URL, JWKSURL: server.URL + "/jwks", Audience: "videohelper"}
	verifier, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	claims := validClaims(cfg)
	valid := signToken(t, first, "first", claims)
	got, err := verifier.Verify(context.Background(), valid)
	if err != nil || got != (Claims{Sub: "subject", Email: "person@example.test", Name: "Person"}) {
		t.Fatalf("claims=%+v error=%v", got, err)
	}
	if _, err := verifier.Verify(context.Background(), valid); err != nil || requests.Load() != 1 {
		t.Fatalf("cached key requests=%d error=%v", requests.Load(), err)
	}
	for name, change := range map[string]func(map[string]any){
		"issuer":      func(c map[string]any) { c["iss"] = "https://wrong.example.test" },
		"audience":    func(c map[string]any) { c["aud"] = "another-service" },
		"expired":     func(c map[string]any) { c["exp"] = time.Now().Add(-time.Hour).Unix() },
		"missing exp": func(c map[string]any) { delete(c, "exp") },
		"missing sub": func(c map[string]any) { delete(c, "sub") },
		"blank sub":   func(c map[string]any) { c["sub"] = "  " },
		"future nbf":  func(c map[string]any) { c["nbf"] = time.Now().Add(time.Hour).Unix() },
	} {
		t.Run(name, func(t *testing.T) {
			c := validClaims(cfg)
			change(c)
			assertRejected(t, verifier, signToken(t, first, "first", c))
		})
	}
	t.Run("tampered", func(t *testing.T) {
		parts := strings.Split(valid, ".")
		body, err := json.Marshal(map[string]any{"iss": cfg.Issuer, "aud": cfg.Audience, "sub": "attacker", "exp": time.Now().Add(time.Hour).Unix()})
		if err != nil {
			t.Fatal(err)
		}
		parts[1] = base64.RawURLEncoding.EncodeToString(body)
		assertRejected(t, verifier, strings.Join(parts, "."))
	})
	t.Run("unsigned and prefixed", func(t *testing.T) {
		parts := strings.Split(valid, ".")
		parts[0] = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
		parts[2] = ""
		assertRejected(t, verifier, strings.Join(parts, "."))
		assertRejected(t, verifier, "Bearer "+valid)
		assertRejected(t, verifier, "")
	})
	keys.Store(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{publicKey(second, "second")}})
	if _, err := verifier.Verify(context.Background(), signToken(t, second, "second", claims)); err != nil {
		t.Fatalf("rotated key: %v", err)
	}
}

func TestVerifyCancellation(t *testing.T) {
	key := rsaKey(t)
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(started) })
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	defer close(release)
	cfg := config.AuthConfig{Issuer: server.URL, JWKSURL: server.URL + "/jwks", Audience: "videohelper"}
	verifier, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	token := signToken(t, key, "key", validClaims(cfg))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := verifier.Verify(ctx, token)
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("JWKS fetch did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || apperr.KindOf(err) != apperr.Unauthenticated || strings.Contains(err.Error(), token) {
			t.Fatalf("cancellation error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("verification did not honor cancellation")
	}
	if _, err := verifier.Verify(ctx, token); !errors.Is(err, context.Canceled) {
		t.Fatalf("already canceled verification=%v", err)
	}
}

func TestNewRejectsInvalidConfiguration(t *testing.T) {
	for _, cfg := range []config.AuthConfig{
		{},
		{Issuer: "https://issuer.example.test", Audience: "aud", JWKSURL: "file:///keys"},
		{Issuer: "https://user:secret@example.test", Audience: "aud", JWKSURL: "https://example.test/keys"},
	} {
		if _, err := New(context.Background(), cfg); err == nil {
			t.Fatalf("invalid config accepted: %+v", cfg)
		}
	}
}

func assertRejected(t *testing.T, verifier *Verifier, token string) {
	t.Helper()
	if _, err := verifier.Verify(context.Background(), token); apperr.KindOf(err) != apperr.Unauthenticated || err == nil || strings.Contains(err.Error(), token) && token != "" {
		t.Fatalf("unsafe/rejected token error=%v", err)
	}
}

func rsaKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func publicKey(key *rsa.PrivateKey, kid string) jose.JSONWebKey {
	return jose.JSONWebKey{Key: &key.PublicKey, KeyID: kid, Algorithm: string(jose.RS256), Use: "sig"}
}

func validClaims(cfg config.AuthConfig) map[string]any {
	return map[string]any{
		"iss": cfg.Issuer, "aud": []string{"another-audience", cfg.Audience}, "sub": "subject", "exp": time.Now().Add(time.Hour).Unix(),
		"email": "person@example.test", "name": "Person", "nbf": time.Now().Add(-time.Minute).Unix(),
	}
}

func signToken(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: key, KeyID: kid}}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	signed, err := signer.Sign(body)
	if err != nil {
		t.Fatal(err)
	}
	token, err := signed.CompactSerialize()
	if err != nil {
		t.Fatal(err)
	}
	return token
}
