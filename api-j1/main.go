// J1 – preuve de concept Arrondi : Enable Banking + BoursoBank.
//
// Usage :
//
//	go run . aspsps            liste les banques FR (filtre sur "bours")
//	go run . auth              démarre l'autorisation, affiche l'URL à ouvrir
//	go run . session <code>    échange le code lu dans l'URL de redirection
//	go run . tx                affiche les transactions brutes (JSON) des 30 derniers jours
//
// Variables d'environnement :
//
//	EB_APP_ID        application_id de la console Enable Banking
//	EB_KEY_PATH      chemin du fichier .pem (clé privée)
//	EB_REDIRECT_URL  l'URL de redirection déclarée dans la console
//	EB_ASPSP_NAME    nom exact de la banque, tel qu'affiché par `aspsps`
package main

import (
	"bytes"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

const (
	apiBase     = "https://api.enablebanking.com"
	sessionFile = "session.json"
)

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "erreur :", err)
		os.Exit(1)
	}
}

func env(k string) string {
	v := os.Getenv(k)
	if v == "" {
		must(fmt.Errorf("variable %s manquante", k))
	}
	return v
}

func loadKey(path string) *rsa.PrivateKey {
	raw, err := os.ReadFile(path)
	must(err)
	block, _ := pem.Decode(raw)
	if block == nil {
		must(fmt.Errorf("%s n'est pas un fichier PEM", path))
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if rk, ok := k.(*rsa.PrivateKey); ok {
			return rk
		}
		must(fmt.Errorf("la clé n'est pas une clé RSA"))
	}
	k, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	must(err)
	return k
}

// token signe un JWT RS256 valable une heure (kid = application_id).
func token() string {
	appID := env("EB_APP_ID")
	key := loadKey(env("EB_KEY_PATH"))
	signer, err := jose.NewSigner(
		jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: key, KeyID: appID}},
		(&jose.SignerOptions{}).WithType("JWT"),
	)
	must(err)
	now := time.Now()
	s, err := jwt.Signed(signer).Claims(map[string]any{
		"iss": "enablebanking.com",
		"aud": "api.enablebanking.com",
		"iat": now.Unix(),
		"exp": now.Add(time.Hour).Unix(),
	}).Serialize()
	must(err)
	return s
}

func call(method, path string, body any) []byte {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		must(err)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, apiBase+path, rd)
	must(err)
	req.Header.Set("Authorization", "Bearer "+token())
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	must(err)
	defer res.Body.Close()
	out, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		must(fmt.Errorf("%s %s -> %d : %s", method, path, res.StatusCode, out))
	}
	return out
}

func pretty(b []byte) string {
	var out bytes.Buffer
	if json.Indent(&out, b, "", "  ") != nil {
		return string(b)
	}
	return out.String()
}

func main() {
	if len(os.Args) < 2 {
		must(fmt.Errorf("commande attendue : aspsps | auth | session <code> | tx"))
	}
	switch os.Args[1] {
	case "aspsps":
		raw := call("GET", "/aspsps?country=FR", nil)
		var r struct {
			ASPSPs []struct {
				Name           string   `json:"name"`
				Country        string   `json:"country"`
				PSUTypes       []string `json:"psu_types"`
				MaximumConsent int      `json:"maximum_consent_validity"`
			} `json:"aspsps"`
		}
		must(json.Unmarshal(raw, &r))
		for _, a := range r.ASPSPs {
			if strings.Contains(strings.ToLower(a.Name), "bours") {
				fmt.Printf("%q  pays=%s  psu=%v  consentement_max=%ds\n", a.Name, a.Country, a.PSUTypes, a.MaximumConsent)
			}
		}
		fmt.Printf("(%d banques FR au total)\n", len(r.ASPSPs))

	case "auth":
		body := map[string]any{
			"access":       map[string]any{"valid_until": time.Now().Add(90 * 24 * time.Hour).UTC().Format(time.RFC3339)},
			"aspsp":        map[string]any{"name": env("EB_ASPSP_NAME"), "country": "FR"},
			"state":        fmt.Sprintf("j1-%d", time.Now().Unix()),
			"redirect_url": env("EB_REDIRECT_URL"),
			"psu_type":     "personal",
		}
		raw := call("POST", "/auth", body)
		var r struct {
			URL string `json:"url"`
		}
		must(json.Unmarshal(raw, &r))
		fmt.Println("Ouvre cette URL dans ton navigateur et autorise l'accès :")
		fmt.Println(r.URL)
		fmt.Println("\nLa page de redirection ne répondra pas : copie la valeur du paramètre `code` dans la barre d'adresse,")
		fmt.Println("puis lance : go run . session <code>")

	case "session":
		if len(os.Args) < 3 {
			must(fmt.Errorf("usage : session <code>"))
		}
		code := os.Args[2]
		if strings.Contains(code, "code=") { // l'URL entière a été collée
			if u, err := url.Parse(code); err == nil {
				code = u.Query().Get("code")
			}
		}
		raw := call("POST", "/sessions", map[string]any{"code": code})
		must(os.WriteFile(sessionFile, raw, 0o600))
		fmt.Println(pretty(raw))
		fmt.Println("\nSession enregistrée dans", sessionFile)

	case "tx":
		raw, err := os.ReadFile(sessionFile)
		must(err)
		var s struct {
			Accounts []json.RawMessage `json:"accounts"`
		}
		must(json.Unmarshal(raw, &s))
		for _, a := range s.Accounts {
			var acc struct {
				UID string `json:"uid"`
			}
			if json.Unmarshal(a, &acc) != nil || acc.UID == "" {
				fmt.Println("compte sans uid, objet brut :", string(a))
				continue
			}
			from := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
			path := fmt.Sprintf("/accounts/%s/transactions?date_from=%s", acc.UID, from)
			fmt.Printf("=== compte %s ===\n", acc.UID)
			fmt.Println(pretty(call("GET", path, nil)))
		}

	default:
		must(fmt.Errorf("commande inconnue : %s", os.Args[1]))
	}
}
