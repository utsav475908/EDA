package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

// OIDCAuthenticator verifies RS256 JWTs issued by Keycloak (or any OIDC IdP)
// against the issuer's JWKS. Keys are cached and refreshed on unknown kid.
type OIDCAuthenticator struct {
	Issuer   string
	Audience string
	JWKSURL  string
	Client   *http.Client
	Now      func() time.Time

	mu          sync.RWMutex
	keys        map[string]*rsa.PublicKey
	lastRefresh time.Time
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

type jwtClaims struct {
	Iss    string   `json:"iss"`
	Sub    string   `json:"sub"`
	Aud    audience `json:"aud"`
	Exp    int64    `json:"exp"`
	Nbf    int64    `json:"nbf"`
	Groups []string `json:"groups"`
	// Keycloak puts the username here; preferred over the opaque sub.
	PreferredUsername string `json:"preferred_username"`
}

// audience accepts both the string and array forms of "aud".
type audience []string

func (a *audience) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*a = []string{s}
		return nil
	}
	var ss []string
	if err := json.Unmarshal(b, &ss); err != nil {
		return err
	}
	*a = ss
	return nil
}

func (o *OIDCAuthenticator) Authenticate(ctx context.Context, token string) (*Principal, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrUnauthenticated
	}
	var hdr jwtHeader
	if err := decodeSegment(parts[0], &hdr); err != nil || hdr.Alg != "RS256" {
		return nil, ErrUnauthenticated
	}
	key, err := o.key(ctx, hdr.Kid)
	if err != nil {
		return nil, ErrUnauthenticated
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrUnauthenticated
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig); err != nil {
		return nil, ErrUnauthenticated
	}
	var c jwtClaims
	if err := decodeSegment(parts[1], &c); err != nil {
		return nil, ErrUnauthenticated
	}
	now := o.now().Unix()
	const skew = 30
	switch {
	case c.Iss != o.Issuer,
		!slices.Contains(c.Aud, o.Audience),
		c.Exp == 0 || now > c.Exp+skew,
		c.Nbf != 0 && now+skew < c.Nbf:
		return nil, ErrUnauthenticated
	}
	sub := c.PreferredUsername
	if sub == "" {
		sub = c.Sub
	}
	// Keycloak's group mapper emits paths like "/tenant:abc"; normalise.
	groups := make([]string, 0, len(c.Groups))
	for _, g := range c.Groups {
		groups = append(groups, strings.TrimPrefix(g, "/"))
	}
	return &Principal{Subject: sub, Groups: groups}, nil
}

func (o *OIDCAuthenticator) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

func (o *OIDCAuthenticator) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	o.mu.RLock()
	k, ok := o.keys[kid]
	fresh := time.Since(o.lastRefresh) < 30*time.Second
	o.mu.RUnlock()
	if ok {
		return k, nil
	}
	if fresh {
		return nil, errors.New("unknown kid")
	}
	if err := o.refresh(ctx); err != nil {
		return nil, err
	}
	o.mu.RLock()
	defer o.mu.RUnlock()
	if k, ok := o.keys[kid]; ok {
		return k, nil
	}
	return nil, errors.New("unknown kid")
}

func (o *OIDCAuthenticator) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.JWKSURL, nil)
	if err != nil {
		return err
	}
	client := o.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks: status %d", resp.StatusCode)
	}
	var set struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return err
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "RSA" {
			continue
		}
		n, err1 := base64.RawURLEncoding.DecodeString(k.N)
		e, err2 := base64.RawURLEncoding.DecodeString(k.E)
		if err1 != nil || err2 != nil {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	o.mu.Lock()
	o.keys, o.lastRefresh = keys, time.Now()
	o.mu.Unlock()
	return nil
}

func decodeSegment(seg string, v any) error {
	b, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
