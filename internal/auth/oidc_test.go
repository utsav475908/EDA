package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func signJWT(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	enc := func(v any) string {
		b, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	signing := enc(map[string]string{"alg": "RS256", "kid": kid, "typ": "JWT"}) + "." + enc(claims)
	digest := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func TestOIDCAuthenticator(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kid": "k1", "kty": "RSA",
			"n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}}})
	}))
	defer jwks.Close()

	now := time.Unix(1_800_000_000, 0)
	a := &OIDCAuthenticator{Issuer: "https://kc/realms/lp", Audience: "lp", JWKSURL: jwks.URL, Now: func() time.Time { return now }}
	base := func() map[string]any {
		return map[string]any{"iss": "https://kc/realms/lp", "aud": []string{"lp", "account"}, "sub": "u-1",
			"preferred_username": "alice", "exp": now.Add(time.Hour).Unix(), "groups": []string{"/tenant:t1"}}
	}

	p, err := a.Authenticate(context.Background(), signJWT(t, key, "k1", base()))
	if err != nil {
		t.Fatalf("valid token rejected: %v", err)
	}
	if p.Subject != "alice" || !p.CanAccessTenant("t1") || p.CanAccessTenant("t2") {
		t.Fatalf("unexpected principal %+v", p)
	}

	bad := map[string]func(map[string]any){
		"expired":      func(c map[string]any) { c["exp"] = now.Add(-time.Hour).Unix() },
		"wrong issuer": func(c map[string]any) { c["iss"] = "https://evil" },
		"wrong aud":    func(c map[string]any) { c["aud"] = "other" },
	}
	for name, mutate := range bad {
		c := base()
		mutate(c)
		if _, err := a.Authenticate(context.Background(), signJWT(t, key, "k1", c)); err == nil {
			t.Errorf("%s: expected rejection", name)
		}
	}

	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	if _, err := a.Authenticate(context.Background(), signJWT(t, other, "k1", base())); err == nil {
		t.Error("token signed by foreign key accepted")
	}
}
