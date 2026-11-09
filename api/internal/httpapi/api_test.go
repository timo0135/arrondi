package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/timotheebenchergui/arrondi/api/internal/app"
	"github.com/timotheebenchergui/arrondi/api/internal/auth"
	"github.com/timotheebenchergui/arrondi/api/internal/bank"
	"github.com/timotheebenchergui/arrondi/api/internal/enablebanking"
	"github.com/timotheebenchergui/arrondi/api/internal/keybox"
)

// --- fausse banque ---

type fakeBank struct {
	items     []enablebanking.Item
	lastState string
	codes     []string
}

func (f *fakeBank) ASPSPs(context.Context, enablebanking.Credentials, string) ([]enablebanking.ASPSP, error) {
	return []enablebanking.ASPSP{{Name: "Boursorama Banque", Country: "FR", MaximumConsentValidity: 15552000}}, nil
}

func (f *fakeBank) StartAuth(_ context.Context, cr enablebanking.Credentials, aspsp, _, redirect, state string, _ time.Time) (string, error) {
	if cr.PrivateKey == nil || cr.ApplicationID != "app-test" {
		return "", enablebanking.ErrCredentials
	}
	f.lastState = state
	return "https://banque.exemple/auth?state=" + state, nil
}

func (f *fakeBank) CreateSession(_ context.Context, _ enablebanking.Credentials, code string) (enablebanking.Session, error) {
	f.codes = append(f.codes, code)
	return enablebanking.Session{
		ID: "sess-1", ValidUntil: time.Now().Add(180 * 24 * time.Hour),
		Accounts: []bank.RawAccount{
			{UID: "uid-cacc", Name: "M TEST", CashAccountType: "CACC", Product: "CAV - BOURSOBANK"},
			{UID: "uid-card", Name: "Carte Visa", CashAccountType: "CARD", Product: "Carte bancaire"},
		},
		IBANs:      map[string]string{"uid-cacc": "FR76 4061 8803 2000 0408 4877 514"},
		Currencies: map[string]string{"uid-cacc": "XXX", "uid-card": "XXX"},
	}, nil
}

func (f *fakeBank) Transactions(_ context.Context, _ enablebanking.Credentials, uid string, _, _ time.Time) ([]enablebanking.Item, error) {
	if uid != "uid-cacc" {
		return nil, nil
	}
	return f.items, nil
}

func item(ref, booked, amount, label string) enablebanking.Item {
	b, _ := json.Marshal(map[string]any{
		"entry_reference": ref, "status": "BOOK", "booking_date": booked, "credit_debit_indicator": "DBIT",
		"transaction_amount": map[string]string{"currency": "EUR", "amount": amount}, "remittance_information": []string{label},
	})
	var tx bank.RawTransaction
	_ = json.Unmarshal(b, &tx)
	return enablebanking.Item{Tx: tx, Raw: b}
}

// --- client HTTP de test ---

type client struct {
	t   *testing.T
	srv *httptest.Server
}

func (c client) do(method, path string, body any, ajax bool) (*http.Response, []byte) {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.srv.URL+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if ajax {
		req.Header.Set("X-Requested-With", "epargne")
	}
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := noRedirect.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	out, _ := io.ReadAll(res.Body)
	return res, out
}

func (c client) expect(method, path string, body any, want int) []byte {
	c.t.Helper()
	res, out := c.do(method, path, body, true)
	if res.StatusCode != want {
		c.t.Fatalf("%s %s : %d, attendu %d — %s", method, path, res.StatusCode, want, out)
	}
	return out
}

func decode[T any](t *testing.T, b []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("JSON %s : %v", b, err)
	}
	return v
}

func TestAPI_ParcoursComplet(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL non défini")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	box, _ := keybox.New(bytes.Repeat([]byte{9}, 32))
	fb := &fakeBank{}
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	a := app.New(pool, box, fb, log)

	email := "admin-" + uuid.NewString()[:8] + "@exemple.fr"
	admin, err := a.EnsureUser(ctx, email, "Admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Q.DeleteUser(ctx, admin.ID)

	s := &Server{App: a, Verifier: auth.DevVerifier{EmailAddr: email}, PublicBaseURL: "https://arrondi.exemple", Log: log}
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	c := client{t, srv}

	// Santé, sans authentification.
	if res, _ := c.do("GET", "/healthz", nil, false); res.StatusCode != 200 {
		t.Fatalf("healthz %d", res.StatusCode)
	}

	// Identité et en-têtes de sécurité.
	res, body := c.do("GET", "/api/v1/me", nil, false)
	if res.StatusCode != 200 || res.Header.Get("Content-Security-Policy") != "default-src 'self'" {
		t.Fatalf("me %d %v", res.StatusCode, res.Header)
	}
	if me := decode[UserOut](t, body); me.Email != email || me.Role != "admin" {
		t.Fatalf("me %+v", me)
	}

	// Tableau de bord vide : pas de banque, pas d'objectif.
	dash := decode[DashboardOut](t, c.expect("GET", "/api/v1/dashboard", nil, 200))
	if dash.Bank != nil || dash.Goal != nil || dash.TotalSavedCents != 0 {
		t.Fatalf("dashboard initial %+v", dash)
	}

	// Une mutation sans X-Requested-With est refusée.
	if res, _ := c.do("PUT", "/api/v1/settings", map[string]any{"step_cents": 100, "multiplier": 1, "tx_cap_cents": 2000}, false); res.StatusCode != 400 {
		t.Fatalf("mutation sans X-Requested-With : %d", res.StatusCode)
	}

	// Réglages : hors plage puis valides.
	c.expect("PUT", "/api/v1/settings", map[string]any{"step_cents": 120, "multiplier": 1, "tx_cap_cents": 2000}, 422)
	st := decode[SettingsBody](t, c.expect("PUT", "/api/v1/settings",
		map[string]any{"step_cents": 100, "multiplier": 2, "tx_cap_cents": 2000, "month_cap_cents": nil}, 200))
	if st.Multiplier != 2 || st.MonthCapCents != nil {
		t.Fatalf("réglages %+v", st)
	}

	// Pas encore de connexion.
	c.expect("GET", "/api/v1/bank/connection", nil, 404)
	c.expect("POST", "/api/v1/bank/authorizations", nil, 404)

	// Identifiants : clé invalide puis valide.
	c.expect("PUT", "/api/v1/bank/credentials", map[string]any{"bank_code": "boursobank", "eb_application_id": "app-test", "private_key_pem": "pas une clé"}, 422)
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	pemStr := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	c.expect("PUT", "/api/v1/bank/credentials", map[string]any{"bank_code": "boursobank", "eb_application_id": "app-test", "private_key_pem": pemStr}, 204)
	conn := decode[ConnectionOut](t, c.expect("GET", "/api/v1/bank/connection", nil, 200))
	if conn.Status != "pending" || conn.ASPSPName != "Boursorama Banque" {
		t.Fatalf("connexion %+v", conn)
	}
	var stored []byte
	pool.QueryRow(ctx, "SELECT eb_private_key_enc FROM bank_connections WHERE user_id=$1", admin.ID).Scan(&stored)
	if bytes.Contains(stored, []byte("PRIVATE KEY")) {
		t.Fatal("la clé privée est stockée en clair")
	}

	// Autorisation : une seule à la fois.
	authOut := decode[struct {
		URL string `json:"url"`
	}](t, c.expect("POST", "/api/v1/bank/authorizations", nil, 200))
	if !strings.HasPrefix(authOut.URL, "https://banque.exemple/auth") {
		t.Fatalf("url %s", authOut.URL)
	}
	c.expect("POST", "/api/v1/bank/authorizations", nil, 409)

	// Retour de la banque : état inconnu, puis le bon.
	c.expect("GET", "/api/v1/bank/callback?state=faux&code=x", nil, 400)
	res, _ = c.do("GET", "/api/v1/bank/callback?state="+url.QueryEscape(fb.lastState)+"&code=code-1", nil, false)
	if res.StatusCode != 302 || res.Header.Get("Location") != "https://arrondi.exemple/reglages/banque?status=ok" {
		t.Fatalf("callback %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	// L'état ne sert qu'une fois.
	c.expect("GET", "/api/v1/bank/callback?state="+url.QueryEscape(fb.lastState)+"&code=code-1", nil, 400)

	conn = decode[ConnectionOut](t, c.expect("GET", "/api/v1/bank/connection", nil, 200))
	if conn.Status != "active" || len(conn.Accounts) != 2 || conn.ActivatedAt == nil {
		t.Fatalf("connexion active %+v", conn)
	}
	var tracked AccountOut
	for _, acc := range conn.Accounts {
		if acc.IsTracked {
			tracked = acc
		}
	}
	if tracked.IBANLast4 == nil || *tracked.IBANLast4 != "7514" || tracked.Currency != "EUR" {
		t.Fatalf("compte suivi automatiquement : %+v", tracked)
	}

	// Synchro manuelle : 202, puis traitée en arrière-plan.
	today := time.Now().Format(dateFmt)
	fb.items = []enablebanking.Item{
		item("1", today, "69.37", "CARTE TOTAL"),      // 0,63 × 2 = 1,26
		item("2", today, "35.77", "CARTE LECLERC"),    // 0,23 × 2 = 0,46
		item("3", today, "105.42", "PRLV SEPA MACIF"), // exclue
	}
	c.expect("POST", "/api/v1/bank/sync", nil, 202)
	deadline := time.Now().Add(5 * time.Second)
	for {
		var n int
		pool.QueryRow(ctx, "SELECT count(*) FROM sync_runs r JOIN bank_connections c ON c.id=r.connection_id WHERE c.user_id=$1 AND r.status='success'", admin.ID).Scan(&n)
		if n == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("la synchro ne s'est pas terminée")
		}
		time.Sleep(50 * time.Millisecond)
	}
	c.expect("POST", "/api/v1/bank/sync", nil, 429) // 1 par heure

	dash = decode[DashboardOut](t, c.expect("GET", "/api/v1/dashboard", nil, 200))
	if dash.TotalSavedCents != 172 || dash.MonthSavedCents != 172 || dash.ToTransferCents != 172 || dash.Bank == nil || dash.Bank.Status != "active" {
		t.Fatalf("dashboard après synchro %+v", dash)
	}

	// Transactions paginées.
	p1 := decode[PageOut[TransactionOut]](t, c.expect("GET", "/api/v1/transactions?limit=2", nil, 200))
	if len(p1.Items) != 2 || p1.NextCursor == nil {
		t.Fatalf("page 1 %+v", p1)
	}
	p2 := decode[PageOut[TransactionOut]](t, c.expect("GET", "/api/v1/transactions?limit=2&cursor="+*p1.NextCursor, nil, 200))
	if len(p2.Items) != 1 || p2.NextCursor != nil {
		t.Fatalf("page 2 %+v", p2)
	}
	all := append(p1.Items, p2.Items...)
	rounded := 0
	for _, tx := range all {
		if tx.Rounding != nil {
			rounded++
		} else if tx.ExclusionReason == nil || *tx.ExclusionReason != "not_card" {
			t.Errorf("transaction non arrondie sans raison : %+v", tx)
		}
	}
	if rounded != 2 {
		t.Errorf("%d transactions arrondies, attendu 2", rounded)
	}
	c.expect("GET", "/api/v1/transactions?month=11-2026", nil, 422)
	if pm := decode[PageOut[TransactionOut]](t, c.expect("GET", "/api/v1/transactions?month="+time.Now().Format("2006-01"), nil, 200)); len(pm.Items) != 3 {
		t.Errorf("filtre par mois : %d", len(pm.Items))
	}

	// Virements et objectif.
	c.expect("POST", "/api/v1/transfers", map[string]any{"amount_cents": 100, "transferred_on": time.Now().AddDate(0, 0, 2).Format(dateFmt)}, 422)
	tr := decode[TransferOut](t, c.expect("POST", "/api/v1/transfers", map[string]any{"amount_cents": 100, "transferred_on": today}, 201))
	c.expect("PUT", "/api/v1/goal", map[string]any{"name": "Apport", "target_cents": 1000}, 200)
	dash = decode[DashboardOut](t, c.expect("GET", "/api/v1/dashboard", nil, 200))
	if dash.TransferredCents != 100 || dash.ToTransferCents != 72 || dash.Goal == nil || dash.Goal.ProgressPercent != 17 {
		t.Fatalf("dashboard avec virement et objectif %+v / %+v", dash, dash.Goal)
	}
	months := decode[[]MonthOut](t, c.expect("GET", "/api/v1/months", nil, 200))
	if len(months) != 1 || months[0].SavedCents != 172 || months[0].TransferredCents != 100 || months[0].TransactionsCount != 2 {
		t.Fatalf("mois %+v", months)
	}
	c.expect("DELETE", "/api/v1/transfers/"+uuid.NewString(), nil, 404)
	c.expect("DELETE", "/api/v1/transfers/"+tr.ID.String(), nil, 204)

	// Administration.
	newEmail := "membre-" + uuid.NewString()[:8] + "@exemple.fr"
	created := decode[AdminUserOut](t, c.expect("POST", "/api/v1/admin/users", map[string]any{"email": newEmail, "display_name": "Membre"}, 201))
	defer a.Q.DeleteUser(ctx, created.ID)
	c.expect("POST", "/api/v1/admin/users", map[string]any{"email": newEmail, "display_name": "Membre"}, 409)
	c.expect("PATCH", "/api/v1/admin/users/"+admin.ID.String(), map[string]any{"is_active": false}, 409)

	// Le membre n'a pas accès à l'administration ; un inconnu est refusé.
	sm := &Server{App: a, Verifier: auth.DevVerifier{EmailAddr: newEmail}, PublicBaseURL: "https://arrondi.exemple", Log: log}
	srvM := httptest.NewServer(sm.Handler())
	defer srvM.Close()
	client{t, srvM}.expect("GET", "/api/v1/admin/users", nil, 403)
	su := &Server{App: a, Verifier: auth.DevVerifier{EmailAddr: "inconnu@exemple.fr"}, PublicBaseURL: "x", Log: log}
	srvU := httptest.NewServer(su.Handler())
	defer srvU.Close()
	client{t, srvU}.expect("GET", "/api/v1/me", nil, 403)

	// Export et suppression du compte.
	res, body = c.do("GET", "/api/v1/me/export", nil, false)
	exp := decode[ExportBody](t, body)
	if res.StatusCode != 200 || !strings.Contains(res.Header.Get("Content-Disposition"), "attachment") || len(exp.Transactions) != 3 {
		t.Fatalf("export %d, %d transactions", res.StatusCode, len(exp.Transactions))
	}
	c.expect("DELETE", "/api/v1/me", map[string]any{"confirm": false}, 422)

	// Route de dev : absente sans Dev, reprend l'historique avec Dev.
	c.expect("POST", "/api/v1/dev/backfill", map[string]any{"days": 30}, 404)
	sd := &Server{App: a, Verifier: auth.DevVerifier{EmailAddr: email}, PublicBaseURL: "https://arrondi.exemple", Dev: true, Log: log}
	srvD := httptest.NewServer(sd.Handler())
	defer srvD.Close()
	cd := client{t, srvD}
	cd.expect("POST", "/api/v1/dev/backfill", map[string]any{"days": 91}, 422)
	fb.items = append(fb.items, item("4", time.Now().AddDate(0, 0, -10).Format(dateFmt), "12.30", "CARTE BOULANGERIE")) // 0,70 × 2 = 1,40
	bf := decode[struct {
		Inserted, Rounded int
		SavedCents        int64 `json:"saved_cents"`
	}](t, cd.expect("POST", "/api/v1/dev/backfill", map[string]any{"days": 30}, 200))
	if bf.Inserted != 4 || bf.Rounded != 3 || bf.SavedCents != 312 {
		t.Fatalf("backfill %+v", bf)
	}

	// Version ordinateur (EF-38 à EF-44).
	month := time.Now().Format("2006-01")
	ms := decode[MonthStatsOut](t, c.expect("GET", "/api/v1/stats/month?month="+month, nil, 200))
	found := map[string]int64{}
	for _, m := range ms.TopMerchants {
		found[m.Merchant] = m.SavedCents
	}
	if found["Carte Total"] != 126 || found["Carte Leclerc"] != 46 || ms.ExcludedByReason["not_card"] != 1 || ms.RoundedCount < 2 {
		t.Fatalf("stats du mois %+v", ms)
	}
	c.expect("GET", "/api/v1/stats/month?month=2026-13", nil, 422)
	ex := decode[PageOut[TransactionOut]](t, c.expect("GET", "/api/v1/transactions?status=excluded&q=m%C3%A2cif", nil, 200))
	if len(ex.Items) != 1 || ex.Items[0].Merchant != "Macif" {
		t.Fatalf("filtre exclues + recherche %+v", ex.Items)
	}
	if r := decode[PageOut[TransactionOut]](t, c.expect("GET", "/api/v1/transactions?status=rounded&q=macif", nil, 200)); len(r.Items) != 0 {
		t.Fatalf("filtre arrondies %+v", r.Items)
	}
	c.expect("GET", "/api/v1/transactions?q=x", nil, 422)
	c.expect("GET", "/api/v1/transactions?q=100%25", nil, 200) // % échappé : aucune erreur SQL
	one := decode[PageOut[TransactionOut]](t, c.expect("GET", "/api/v1/transactions?q=total", nil, 200))
	if len(one.Items) != 1 {
		t.Fatalf("recherche total %+v", one.Items)
	}
	d := decode[TransactionOut](t, c.expect("GET", "/api/v1/transactions/"+one.Items[0].ID.String(), nil, 200))
	if d.Rounding == nil || d.Rounding.StepCents != 100 || d.Rounding.Multiplier != 2 || d.Rounding.FinalCents != 126 {
		t.Fatalf("détail %+v / %+v", d, d.Rounding)
	}
	c.expect("GET", "/api/v1/transactions/"+uuid.NewString(), nil, 404)
	res, body = c.do("GET", "/api/v1/transactions/export?month="+month, nil, false)
	if res.StatusCode != 200 || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/csv") ||
		!bytes.HasPrefix(body, []byte("\xEF\xBB\xBF")) || !bytes.Contains(body, []byte("Carte Total;-69,37;1,26;")) {
		t.Fatalf("export CSV %d %q", res.StatusCode, body)
	}
	pv := decode[PreviewOut](t, c.expect("GET", "/api/v1/settings/preview?step_cents=100&multiplier=2&tx_cap_cents=2000", nil, 200))
	if pv.Example.AmountCents != 2340 || pv.Example.BaseCents != 60 || pv.Example.MultipliedCents != 120 || pv.Example.FinalCents != 120 {
		t.Fatalf("aperçu %+v", pv)
	}
	c.expect("GET", "/api/v1/settings/preview?step_cents=33&multiplier=2&tx_cap_cents=2000", nil, 422)
	if runs := decode[[]SyncRunOut](t, c.expect("GET", "/api/v1/bank/sync-runs", nil, 200)); len(runs) == 0 || runs[0].Status != "success" {
		t.Fatalf("synchros %+v", runs)
	}
	g := decode[GoalOut](t, c.expect("GET", "/api/v1/goal", nil, 200))
	if g.SavedCents != 312 || g.MonthlyPaceCents != nil || g.EstimatedMonth != nil {
		t.Fatalf("objectif et projection %+v", g)
	}
	var me *AdminUserOut
	for _, u := range decode[[]AdminUserOut](t, c.expect("GET", "/api/v1/admin/users", nil, 200)) {
		if u.ID == admin.ID {
			me = &u
		}
	}
	if me == nil || me.LastSyncOkAt == nil || me.ConsentValidUntil == nil {
		t.Fatalf("admin : dernière synchro %+v", me)
	}

	// Suppression de la connexion : tout part en cascade.
	c.expect("DELETE", "/api/v1/bank/connection", nil, 204)
	if d := decode[DashboardOut](t, c.expect("GET", "/api/v1/dashboard", nil, 200)); d.TotalSavedCents != 0 || d.Bank != nil {
		t.Fatalf("après suppression de la connexion %+v", d)
	}

	// L'OpenAPI est généré.
	if oa := c.expect("GET", "/api/v1/openapi.json", nil, 200); !bytes.Contains(oa, []byte(`"/bank/sync"`)) {
		t.Error("OpenAPI incomplet")
	}
}

func TestRateLimiter(t *testing.T) {
	l := newRateLimiter(3, time.Minute)
	now := time.Now()
	l.now = func() time.Time { return now }
	for i := 0; i < 3; i++ {
		if !l.allow("u") {
			t.Fatal("refus trop tôt")
		}
	}
	if l.allow("u") {
		t.Fatal("4e requête acceptée")
	}
	if !l.allow("v") {
		t.Fatal("les utilisateurs doivent être indépendants")
	}
	now = now.Add(time.Minute)
	if !l.allow("u") {
		t.Fatal("la fenêtre aurait dû repartir")
	}
}
