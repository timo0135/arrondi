// Package enablebanking est le client HTTP de l'API Enable Banking
// (https://api.enablebanking.com), limité à ce dont Arrondi a besoin.
//
// Chaque utilisateur a sa propre application Enable Banking (D-01) : les
// identifiants (application_id + clé privée) sont passés à chaque appel.
package enablebanking

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/timotheebenchergui/arrondi/api/internal/bank"
)

const DefaultBaseURL = "https://api.enablebanking.com"

// Credentials identifie l'application Enable Banking d'un utilisateur.
type Credentials struct {
	ApplicationID string
	PrivateKey    *rsa.PrivateKey
}

// Erreurs classées selon le cahier des charges, section 9.4.
var (
	ErrConsent     = errors.New("enablebanking : consentement expiré ou révoqué")                // → statut expired
	ErrCredentials = errors.New("enablebanking : identifiant ou clé refusés par Enable Banking") // → statut error
	ErrRateLimited = errors.New("enablebanking : trop de lectures (429)")                        // → pas de nouvelle tentative
	ErrTemporary   = errors.New("enablebanking : erreur temporaire")                             // → nouvelles tentatives
	ErrBadResponse = errors.New("enablebanking : réponse inattendue ou illisible")               // → statut error
)

// Client appelle l'API. Le zéro n'est pas utilisable : passer par New.
type Client struct {
	baseURL string
	http    *http.Client
	now     func() time.Time
}

// New crée un client avec un délai de 30 secondes par requête (section 9.4).
func New(baseURL string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 30 * time.Second},
		now:     time.Now,
	}
}

// ParsePrivateKey lit une clé privée RSA au format PEM (PKCS#8 ou PKCS#1),
// telle que fournie par la console Enable Banking.
func ParsePrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("clé privée : contenu PEM introuvable")
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rk, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("clé privée : ce n'est pas une clé RSA")
		}
		return rk, nil
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

func (c *Client) token(cr Credentials) (string, error) {
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: cr.PrivateKey, KeyID: cr.ApplicationID}},
		(&jose.SignerOptions{}).WithType("JWT"),
	)
	if err != nil {
		return "", err
	}
	now := c.now()
	return jwt.Signed(signer).Claims(map[string]any{
		"iss": "enablebanking.com",
		"aud": "api.enablebanking.com",
		"iat": now.Unix(),
		"exp": now.Add(time.Hour).Unix(),
	}).Serialize()
}

func (c *Client) do(ctx context.Context, cr Credentials, method, path string, body, out any) error {
	tok, err := c.token(cr)
	if err != nil {
		return fmt.Errorf("signature du JWT : %w", err)
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w : %v", ErrTemporary, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 10<<20))
	if err != nil {
		return fmt.Errorf("%w : lecture de la réponse : %v", ErrTemporary, err)
	}
	if err := classify(res.StatusCode, raw); err != nil {
		return err
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("%w : %v", ErrBadResponse, err)
		}
	}
	return nil
}

// classify traduit un code HTTP en erreur de la section 9.4. Le corps n'est
// jamais recopié dans l'erreur : il peut contenir des données personnelles.
func classify(status int, body []byte) error {
	switch {
	case status < 300:
		return nil
	case status == http.StatusTooManyRequests:
		return ErrRateLimited
	case status >= 500:
		return fmt.Errorf("%w : HTTP %d", ErrTemporary, status)
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		// À confirmer sur un vrai cas d'expiration : on considère qu'un refus qui
		// mentionne la session ou le consentement vient de la banque, et qu'un autre
		// refus vient du JWT (application_id ou clé).
		var e struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &e)
		code := strings.ToUpper(e.Error + " " + e.Message)
		if status == http.StatusForbidden || strings.Contains(code, "SESSION") || strings.Contains(code, "CONSENT") {
			return ErrConsent
		}
		return ErrCredentials
	default:
		return fmt.Errorf("%w : HTTP %d", ErrBadResponse, status)
	}
}

// Item est une transaction lue : la structure décodée et le JSON brut (stocké dans transactions.raw).
type Item struct {
	Tx  bank.RawTransaction
	Raw json.RawMessage
}

// Transactions lit toutes les transactions comptabilisées d'un compte entre
// deux dates incluses, en suivant continuation_key jusqu'à la dernière page.
func (c *Client) Transactions(ctx context.Context, cr Credentials, accountUID string, from, to time.Time) ([]Item, error) {
	var items []Item
	key := ""
	for page := 0; ; page++ {
		if page >= 100 {
			return nil, fmt.Errorf("%w : plus de 100 pages", ErrBadResponse)
		}
		q := url.Values{}
		q.Set("date_from", from.Format("2006-01-02"))
		q.Set("date_to", to.Format("2006-01-02"))
		q.Set("transaction_status", "BOOK")
		if key != "" {
			q.Set("continuation_key", key)
		}
		var res struct {
			Transactions    []json.RawMessage `json:"transactions"`
			ContinuationKey *string           `json:"continuation_key"`
		}
		path := "/accounts/" + url.PathEscape(accountUID) + "/transactions?" + q.Encode()
		if err := c.do(ctx, cr, http.MethodGet, path, nil, &res); err != nil {
			return nil, err
		}
		for _, raw := range res.Transactions {
			var tx bank.RawTransaction
			if err := json.Unmarshal(raw, &tx); err != nil {
				return nil, fmt.Errorf("%w : transaction illisible : %v", ErrBadResponse, err)
			}
			items = append(items, Item{Tx: tx, Raw: raw})
		}
		if res.ContinuationKey == nil || *res.ContinuationKey == "" {
			return items, nil
		}
		key = *res.ContinuationKey
	}
}
