package authn

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/ozine/videohelper/internal/apperr"
	"github.com/ozine/videohelper/internal/config"
)

type Verifier struct {
	tokens *oidc.IDTokenVerifier
}

// New uses the configured JWKS endpoint directly; keys are fetched lazily and cached.
func New(ctx context.Context, cfg config.AuthConfig) (*Verifier, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !authURL(cfg.Issuer) || !authURL(cfg.JWKSURL) || strings.TrimSpace(cfg.Audience) == "" {
		return nil, errors.New("auth requires valid issuer, JWKS URL, and audience")
	}
	keyCtx := oidc.ClientContext(ctx, &http.Client{Timeout: 10 * time.Second})
	keys := oidc.NewRemoteKeySet(keyCtx, cfg.JWKSURL)
	return &Verifier{tokens: oidc.NewVerifier(cfg.Issuer, keys, &oidc.Config{
		ClientID: cfg.Audience, SupportedSigningAlgs: []string{oidc.RS256},
	})}, nil
}

// Verify accepts the raw JWT; the HTTP entry point parses the Authorization Bearer prefix.
// The OIDC verifier applies its standard five-minute clock-skew allowance to nbf.
func (v *Verifier) Verify(ctx context.Context, bearer string) (Claims, error) {
	if err := ctx.Err(); err != nil {
		return Claims{}, apperr.Wrap(apperr.Unauthenticated, "unauthenticated", err)
	}
	token, err := v.tokens.Verify(ctx, bearer)
	if err := ctx.Err(); err != nil {
		return Claims{}, apperr.Wrap(apperr.Unauthenticated, "unauthenticated", err)
	}
	if err != nil {
		return Claims{}, apperr.New(apperr.Unauthenticated, "unauthenticated", "令牌无效或已过期")
	}
	var profile struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if strings.TrimSpace(token.Subject) == "" || token.Claims(&profile) != nil {
		return Claims{}, apperr.New(apperr.Unauthenticated, "unauthenticated", "令牌无效或已过期")
	}
	return Claims{Sub: token.Subject, Email: profile.Email, Name: profile.Name}, nil
}

func authURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == ""
}
