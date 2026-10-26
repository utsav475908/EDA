// Package auth authenticates API callers and answers coarse tenancy checks.
// Fine-grained data permissions live in Ranger (Phase 2); this package only
// decides who may touch which tenant's control-plane objects.
package auth

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
)

const (
	// AdminGroup members can manage every tenant.
	AdminGroup = "lp-admins"
	// TenantGroupPrefix: membership in "tenant:<id>" grants access to that tenant.
	TenantGroupPrefix = "tenant:"
)

var ErrUnauthenticated = errors.New("unauthenticated")

type Principal struct {
	Subject string   `json:"subject"`
	Groups  []string `json:"groups"`
}

func (p *Principal) IsAdmin() bool { return slices.Contains(p.Groups, AdminGroup) }

func (p *Principal) CanAccessTenant(tenantID string) bool {
	return p.IsAdmin() || slices.Contains(p.Groups, TenantGroupPrefix+tenantID)
}

type Authenticator interface {
	Authenticate(ctx context.Context, bearer string) (*Principal, error)
}

type ctxKey struct{}

func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

func FromContext(ctx context.Context) (*Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(*Principal)
	return p, ok
}

// Middleware rejects requests without a valid bearer token.
func Middleware(a Authenticator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || tok == "" {
			writeUnauthorized(w)
			return
		}
		p, err := a.Authenticate(r.Context(), tok)
		if err != nil {
			writeUnauthorized(w)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
	})
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", `Bearer realm="lp"`)
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"code":16,"message":"unauthenticated"}`))
}

// DevAuthenticator maps static tokens to principals. Never use in production.
type DevAuthenticator struct {
	tokens map[string]*Principal
}

// NewDevAuthenticator takes token -> "subject:group1,group2".
func NewDevAuthenticator(spec map[string]string) *DevAuthenticator {
	d := &DevAuthenticator{tokens: map[string]*Principal{}}
	for tok, v := range spec {
		sub, groups, _ := strings.Cut(v, ":")
		p := &Principal{Subject: sub}
		for _, g := range strings.Split(groups, ",") {
			if g = strings.TrimSpace(g); g != "" {
				p.Groups = append(p.Groups, g)
			}
		}
		d.tokens[tok] = p
	}
	return d
}

func (d *DevAuthenticator) Authenticate(_ context.Context, bearer string) (*Principal, error) {
	p, ok := d.tokens[bearer]
	if !ok {
		return nil, ErrUnauthenticated
	}
	c := *p
	return &c, nil
}
