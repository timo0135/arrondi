package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

type fakeCF struct {
	srv  *httptest.Server
	key  *rsa.PrivateKey
	kid  string
	hits int
}

func newFakeCF(t *testing.T) *fakeCF {
	f := &fakeCF{kid: "k1"}
	f.key, _ = rsa.GenerateKey(rand.Reader, 2048)
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits++
		json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{
			{Key: &f.key.PublicKey, KeyID: f.kid, Algorithm: "RS256", Use: "sig"},
		}})
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeCF) verifier() *AccessVerifier {
	v, _ := NewAccessVerifier("equipe.cloudflareaccess.com", "aud-arrondi")
	v.certsURL = f.srv.URL
	return v
}

func (f *fakeCF) token(t *testing.T, mut func(*claims)) string {
	signer, _ := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: f.key, KeyID: f.kid}}, nil)
	c := claims{Claims: jwt.Claims{
		Issuer: "https://equipe.cloudflareaccess.com", Audience: jwt.Audience{"aud-arrondi"},
		Expiry: jwt.NewNumericDate(time.Now().Add(time.Hour)), IssuedAt: jwt.NewNumericDate(time.Now()),
	}, Email: "Timothee@Exemple.fr"}
	if mut != nil {
		mut(&c)
	}
	s, err := jwt.Signed(signer).Claims(c).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func req(tok string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	if tok != "" {
		r.Header.Set(Header, tok)
	}
	return r
}

func TestAccessVerifier_Valide(t *testing.T) {
	f := newFakeCF(t)
	v := f.verifier()
	email, err := v.Email(req(f.token(t, nil)))
	if err != nil || email != "timothee@exemple.fr" {
		t.Fatalf("Email = %q, %v", email, err)
	}
	v.Email(req(f.token(t, nil)))
	if f.hits != 1 {
		t.Errorf("clés relues %d fois, attendu 1 (cache)", f.hits)
	}
}

func TestAccessVerifier_Refus(t *testing.T) {
	f := newFakeCF(t)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	cases := map[string]string{
		"absent":     "",
		"illisible":  "pas.un.jwt",
		"expiré":     f.token(t, func(c *claims) { c.Expiry = jwt.NewNumericDate(time.Now().Add(-time.Hour)) }),
		"audience":   f.token(t, func(c *claims) { c.Audience = jwt.Audience{"autre"} }),
		"émetteur":   f.token(t, func(c *claims) { c.Issuer = "https://pirate.cloudflareaccess.com" }),
		"sans email": f.token(t, func(c *claims) { c.Email = "" }),
	}
	// Signé par une autre clé avec le même kid.
	signer, _ := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: other, KeyID: "k1"}}, nil)
	forged, _ := jwt.Signed(signer).Claims(claims{Claims: jwt.Claims{Issuer: "https://equipe.cloudflareaccess.com",
		Audience: jwt.Audience{"aud-arrondi"}, Expiry: jwt.NewNumericDate(time.Now().Add(time.Hour))}, Email: "x@y.fr"}).Serialize()
	cases["fausse signature"] = forged

	v := f.verifier()
	for name, tok := range cases {
		if _, err := v.Email(req(tok)); !errors.Is(err, ErrUnauthenticated) {
			t.Errorf("%s : erreur %v, attendu ErrUnauthenticated", name, err)
		}
	}
}

func TestAccessVerifier_RotationDesCles(t *testing.T) {
	f := newFakeCF(t)
	v := f.verifier()
	v.Email(req(f.token(t, nil)))
	f.key, _ = rsa.GenerateKey(rand.Reader, 2048) // Cloudflare change de clé
	f.kid = "k2"
	if _, err := v.Email(req(f.token(t, nil))); err != nil {
		t.Fatalf("après rotation : %v", err)
	}
	if f.hits != 2 {
		t.Errorf("clés relues %d fois, attendu 2", f.hits)
	}
}

func TestNewAccessVerifier(t *testing.T) {
	if _, err := NewAccessVerifier("", "aud"); err == nil {
		t.Error("domaine vide accepté")
	}
	v, _ := NewAccessVerifier("https://equipe.cloudflareaccess.com/", "aud")
	if !strings.HasSuffix(v.certsURL, "equipe.cloudflareaccess.com/cdn-cgi/access/certs") {
		t.Errorf("certsURL = %s", v.certsURL)
	}
}
