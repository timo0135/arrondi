// Package auth vérifie l'identité transmise par Cloudflare Access
// (cahier des charges, section 10) : JWT de l'en-tête Cf-Access-Jwt-Assertion,
// signature avec les clés publiques de l'équipe (cache 1 heure), audience,
// émetteur et expiration. L'API n'a aucune autre forme d'authentification.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// Header est l'en-tête posé par Cloudflare Access sur chaque requête.
const Header = "Cf-Access-Jwt-Assertion"

var ErrUnauthenticated = errors.New("JWT Cloudflare Access absent ou invalide")

// Verifier renvoie l'email authentifié d'une requête.
type Verifier interface {
	Email(r *http.Request) (string, error)
}

// AccessVerifier vérifie les JWT Cloudflare Access.
type AccessVerifier struct {
	issuer   string // https://<équipe>.cloudflareaccess.com
	audience string
	certsURL string
	client   *http.Client
	now      func() time.Time

	mu      sync.Mutex
	keys    jose.JSONWebKeySet
	fetched time.Time
}

// NewAccessVerifier : teamDomain vaut "<équipe>.cloudflareaccess.com" (avec ou sans https://).
func NewAccessVerifier(teamDomain, audience string) (*AccessVerifier, error) {
	teamDomain = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(teamDomain), "https://"), "/")
	if teamDomain == "" || audience == "" {
		return nil, errors.New("CF_ACCESS_TEAM_DOMAIN et CF_ACCESS_AUD sont obligatoires")
	}
	issuer := "https://" + teamDomain
	return &AccessVerifier{
		issuer: issuer, audience: audience, certsURL: issuer + "/cdn-cgi/access/certs",
		client: &http.Client{Timeout: 10 * time.Second}, now: time.Now,
	}, nil
}

const keysTTL = time.Hour

func (v *AccessVerifier) keySet(ctx context.Context, force bool) (jose.JSONWebKeySet, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !force && len(v.keys.Keys) > 0 && v.now().Sub(v.fetched) < keysTTL {
		return v.keys, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.certsURL, nil)
	if err != nil {
		return jose.JSONWebKeySet{}, err
	}
	res, err := v.client.Do(req)
	if err != nil {
		return v.keys, fmt.Errorf("clés Cloudflare Access : %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return v.keys, fmt.Errorf("clés Cloudflare Access : HTTP %d", res.StatusCode)
	}
	var set jose.JSONWebKeySet
	if err := json.NewDecoder(res.Body).Decode(&set); err != nil {
		return v.keys, fmt.Errorf("clés Cloudflare Access : %w", err)
	}
	v.keys, v.fetched = set, v.now()
	return set, nil
}

type claims struct {
	jwt.Claims
	Email string `json:"email"`
}

func (v *AccessVerifier) Email(r *http.Request) (string, error) {
	raw := r.Header.Get(Header)
	if raw == "" {
		return "", ErrUnauthenticated
	}
	tok, err := jwt.ParseSigned(raw, []jose.SignatureAlgorithm{jose.RS256})
	if err != nil || len(tok.Headers) == 0 {
		return "", ErrUnauthenticated
	}
	kid := tok.Headers[0].KeyID

	var c claims
	verify := func(set jose.JSONWebKeySet) bool {
		for _, k := range set.Key(kid) {
			if tok.Claims(k.Key, &c) == nil {
				return true
			}
		}
		return false
	}
	set, err := v.keySet(r.Context(), false)
	if err != nil && len(set.Keys) == 0 {
		return "", err
	}
	if !verify(set) {
		// Rotation des clés : on relit le jeu une fois.
		if set, err = v.keySet(r.Context(), true); err != nil || !verify(set) {
			return "", ErrUnauthenticated
		}
	}
	if err := c.Claims.ValidateWithLeeway(jwt.Expected{
		Issuer: v.issuer, AnyAudience: jwt.Audience{v.audience}, Time: v.now(),
	}, time.Minute); err != nil {
		return "", ErrUnauthenticated
	}
	if c.Email == "" {
		return "", ErrUnauthenticated
	}
	return strings.ToLower(c.Email), nil
}

// DevVerifier est réservé au développement local : il renvoie toujours le même
// email, sans Cloudflare. Il n'est activé que si APP_ENV=dev (voir config).
type DevVerifier struct{ EmailAddr string }

func (d DevVerifier) Email(*http.Request) (string, error) { return strings.ToLower(d.EmailAddr), nil }
