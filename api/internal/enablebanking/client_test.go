package enablebanking

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testCreds(t *testing.T) Credentials {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return Credentials{ApplicationID: "app-test", PrivateKey: k}
}

func TestTransactions_PaginationEtParametres(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ey") {
			t.Errorf("pas de JWT : %q", r.Header.Get("Authorization"))
		}
		if r.URL.Path != "/accounts/uid-1/transactions" {
			t.Errorf("chemin %s", r.URL.Path)
		}
		queries = append(queries, r.URL.RawQuery)
		if r.URL.Query().Get("continuation_key") == "" {
			w.Write([]byte(`{"transactions":[{"entry_reference":"1","status":"BOOK","booking_date":"2026-11-05",
				"credit_debit_indicator":"DBIT","transaction_amount":{"currency":"EUR","amount":"69.37"},
				"remittance_information":["CARTE TOTAL"]}],"continuation_key":"page2"}`))
			return
		}
		w.Write([]byte(`{"transactions":[{"entry_reference":"2","status":"BOOK","booking_date":"2026-11-04",
			"credit_debit_indicator":"DBIT","transaction_amount":{"currency":"EUR","amount":"5"},
			"remittance_information":["CARTE X"]}],"continuation_key":null}`))
	}))
	defer srv.Close()

	c := New(srv.URL)
	from := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	items, err := c.Transactions(context.Background(), testCreds(t), "uid-1", from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Tx.EntryReference != "1" || items[1].Tx.EntryReference != "2" {
		t.Fatalf("items %+v", items)
	}
	if len(items[0].Raw) == 0 {
		t.Error("JSON brut non conservé")
	}
	if len(queries) != 2 || !strings.Contains(queries[0], "date_from=2026-10-29") ||
		!strings.Contains(queries[0], "date_to=2026-11-08") || !strings.Contains(queries[0], "transaction_status=BOOK") ||
		!strings.Contains(queries[1], "continuation_key=page2") {
		t.Errorf("requêtes %v", queries)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   error
	}{
		{200, ``, nil},
		{429, ``, ErrRateLimited},
		{502, ``, ErrTemporary},
		{403, ``, ErrConsent},
		{401, `{"error":"EXPIRED_SESSION"}`, ErrConsent},
		{401, `{"error":"INVALID_JWT","message":"bad signature"}`, ErrCredentials},
		{422, `{"error":"WRONG_AUTHORIZATION_CODE"}`, ErrBadResponse},
	}
	for _, c := range cases {
		got := classify(c.status, []byte(c.body))
		if c.want == nil && got != nil || c.want != nil && !errors.Is(got, c.want) {
			t.Errorf("classify(%d, %s) = %v, attendu %v", c.status, c.body, got, c.want)
		}
	}
}

func TestErreurReseauTemporaire(t *testing.T) {
	c := New("http://127.0.0.1:1") // port fermé
	_, err := c.Transactions(context.Background(), testCreds(t), "x", time.Now(), time.Now())
	if !errors.Is(err, ErrTemporary) {
		t.Errorf("erreur %v, attendu ErrTemporary", err)
	}
}

func TestReponseIllisible(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`pas du json`)) }))
	defer srv.Close()
	_, err := New(srv.URL).Transactions(context.Background(), testCreds(t), "x", time.Now(), time.Now())
	if !errors.Is(err, ErrBadResponse) {
		t.Errorf("erreur %v, attendu ErrBadResponse", err)
	}
}
