package syncer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/timotheebenchergui/arrondi/api/internal/bank"
	"github.com/timotheebenchergui/arrondi/api/internal/enablebanking"
	"github.com/timotheebenchergui/arrondi/api/internal/rounding"
)

// --- fausse base, en mémoire, qui respecte l'unicité (account_id, external_id) ---

type fakeStore struct {
	settings   rounding.Settings
	txs        map[string]NewTransaction // clé : external_id
	ids        map[string]uuid.UUID
	roundings  []NewRounding
	runs       []string // "trigger:status:fetched:inserted"
	status     string
	lastErr    string
	failInsert bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{settings: rounding.DefaultSettings(), txs: map[string]NewTransaction{}, ids: map[string]uuid.UUID{}, status: "active"}
}

func (f *fakeStore) Settings(context.Context, uuid.UUID) (rounding.Settings, error) {
	return f.settings, nil
}

func (f *fakeStore) LastBookedOn(_ context.Context, _ uuid.UUID) (time.Time, bool, error) {
	var last time.Time
	for _, t := range f.txs {
		if t.BookedOn.After(last) {
			last = t.BookedOn
		}
	}
	return last, !last.IsZero(), nil
}

func (f *fakeStore) StartRun(context.Context, uuid.UUID, string) (uuid.UUID, error) {
	return uuid.New(), nil
}

func (f *fakeStore) FinishRun(_ context.Context, _ uuid.UUID, status string, fetched, inserted int, _ string) error {
	f.runs = append(f.runs, fmt.Sprintf("%s:%d:%d", status, fetched, inserted))
	return nil
}

func (f *fakeStore) RecordAttempt(_ context.Context, _ uuid.UUID, ok bool, msg string) error {
	f.lastErr = msg
	return nil
}

func (f *fakeStore) SetConnectionStatus(_ context.Context, _ uuid.UUID, status, msg string) error {
	f.status = status
	return nil
}

// InUserTx simule une transaction : copie, puis remplace seulement si tout réussit.
func (f *fakeStore) InUserTx(ctx context.Context, _ uuid.UUID, fn func(Tx) error) error {
	tx := &fakeTx{parent: f, txs: map[string]NewTransaction{}, ids: map[string]uuid.UUID{}}
	for k, v := range f.txs {
		tx.txs[k], tx.ids[k] = v, f.ids[k]
	}
	tx.roundings = append(tx.roundings, f.roundings...)
	if err := fn(tx); err != nil {
		return err
	}
	f.txs, f.ids, f.roundings = tx.txs, tx.ids, tx.roundings
	return nil
}

type fakeTx struct {
	parent    *fakeStore
	txs       map[string]NewTransaction
	ids       map[string]uuid.UUID
	roundings []NewRounding
}

func (t *fakeTx) InsertTransaction(_ context.Context, n NewTransaction) (uuid.UUID, bool, error) {
	if t.parent.failInsert && n.ExternalID == "boom" {
		return uuid.Nil, false, errors.New("panne base")
	}
	if _, ok := t.txs[n.ExternalID]; ok {
		return uuid.Nil, false, nil
	}
	id := uuid.New()
	t.txs[n.ExternalID], t.ids[n.ExternalID] = n, id
	return id, true, nil
}

func (t *fakeTx) MonthToDateCents(_ context.Context, _ uuid.UUID, month time.Time) (int64, error) {
	var sum int64
	for _, r := range t.roundings {
		if r.Month.Equal(month) {
			sum += r.Result.FinalCents
		}
	}
	return sum, nil
}

func (t *fakeTx) InsertRounding(_ context.Context, r NewRounding) error {
	t.roundings = append(t.roundings, r)
	return nil
}

// --- fausse banque ---

type fakeBank struct {
	items   []enablebanking.Item
	err     error
	gotFrom time.Time
	gotTo   time.Time
	calls   int
}

func (b *fakeBank) Transactions(_ context.Context, _ enablebanking.Credentials, _ string, from, to time.Time) ([]enablebanking.Item, error) {
	b.calls++
	b.gotFrom, b.gotTo = from, to
	return b.items, b.err
}

func item(t *testing.T, ref, status, booked, amount, side, label string) enablebanking.Item {
	t.Helper()
	m := map[string]any{
		"entry_reference": ref, "status": status, "credit_debit_indicator": side,
		"transaction_amount":     map[string]string{"currency": "EUR", "amount": amount},
		"remittance_information": []string{label}, "transaction_id": nil,
	}
	if booked != "" {
		m["booking_date"] = booked
	} else {
		m["booking_date"] = nil
	}
	raw, _ := json.Marshal(m)
	var tx bank.RawTransaction
	if err := json.Unmarshal(raw, &tx); err != nil {
		t.Fatal(err)
	}
	return enablebanking.Item{Tx: tx, Raw: raw}
}

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, paris)
	if err != nil {
		panic(err)
	}
	return t
}

func newService(st *fakeStore, b *fakeBank, now string) (*Service, Connection) {
	s := &Service{Store: st, Bank: b, Profiles: []bank.Profile{bank.BoursoBank{}}, Now: func() time.Time { return at(now) }}
	c := Connection{ID: uuid.New(), UserID: uuid.New(), BankCode: "boursobank",
		ActivatedAt: at("2026-10-20 18:00"), AccountID: uuid.New(), AccountUID: "uid-cacc"}
	return s, c
}

// Jeu inspiré du J1 : 3 cartes, 1 prélèvement, 1 crédit, 1 en attente, 1 avant activation.
func j1Items(t *testing.T) []enablebanking.Item {
	return []enablebanking.Item{
		item(t, "avis-1", "PDNG", "", "24", "DBIT", "100% COIFFURE\\BREST\\ FR"),
		item(t, "32722526410", "BOOK", "2026-11-05", "69.37", "DBIT", "CARTE 04/11/26 TOTAL 4 CB*6804"),
		item(t, "32710252754", "BOOK", "2026-11-05", "105.42", "DBIT", "PRLV SEPA MACIF"),
		item(t, "32684943033", "BOOK", "2026-11-03", "35.77", "DBIT", "CARTE 02/11/26 LECLERC LANDERNEA CB*6804"),
		item(t, "32561334197", "BOOK", "2026-10-28", "1095.9", "CRDT", "VIR SEPA EMPLOYEUR"),
		item(t, "32549196459", "BOOK", "2026-10-27", "31.27", "DBIT", "CARTE 26/10/26 LECLERC LANDERNEA CB*6804"),
		item(t, "32402952414", "BOOK", "2026-10-19", "128.39", "DBIT", "CARTE 16/10/26 Leboncoin CB*6804"),
	}
}

func TestSync_PremierImport(t *testing.T) {
	st, b := newFakeStore(), &fakeBank{items: j1Items(t)}
	s, c := newService(st, b, "2026-11-08 06:30")

	res, err := s.Sync(context.Background(), c, TriggerScheduled)
	if err != nil {
		t.Fatal(err)
	}
	// Fenêtre : du jour d'activation à aujourd'hui (aucune transaction connue).
	if got := b.gotFrom.Format("2006-01-02"); got != "2026-10-20" {
		t.Errorf("date_from = %s, attendu 2026-10-20", got)
	}
	if got := b.gotTo.Format("2006-01-02"); got != "2026-11-08" {
		t.Errorf("date_to = %s, attendu 2026-11-08", got)
	}
	// 7 lues, 6 comptabilisées enregistrées, 3 cartes arrondies après activation.
	if res.Fetched != 7 || res.Inserted != 6 || res.Rounded != 3 {
		t.Fatalf("résultat %+v, attendu 7 lues / 6 insérées / 3 arrondies", res)
	}
	// 69,37 → 0,63 ; 35,77 → 0,23 ; 31,27 → 0,73
	if res.SavedCents != 63+23+73 {
		t.Errorf("épargne %d c, attendu 159 c", res.SavedCents)
	}
	reasons := map[string]string{
		"32710252754": ReasonNotCard, "32561334197": ReasonCredit,
		"32402952414": ReasonBeforeActivation, "32722526410": "",
	}
	for id, want := range reasons {
		if got := st.txs[id].ExclusionReason; got != want {
			t.Errorf("%s : raison %q, attendu %q", id, got, want)
		}
	}
	if _, ok := st.txs["avis-1"]; ok {
		t.Error("la transaction en attente ne doit pas être enregistrée")
	}
	if st.runs[0] != "success:7:6" {
		t.Errorf("sync_runs = %v", st.runs)
	}
	// Mois de chaque arrondi = mois de la date de comptabilisation.
	for _, r := range st.roundings {
		if r.Month.Day() != 1 {
			t.Errorf("mois %v n'est pas un premier du mois", r.Month)
		}
	}
}

func TestSync_RelectureIdempotente(t *testing.T) {
	st, b := newFakeStore(), &fakeBank{items: j1Items(t)}
	s, c := newService(st, b, "2026-11-08 06:30")
	if _, err := s.Sync(context.Background(), c, TriggerScheduled); err != nil {
		t.Fatal(err)
	}
	res, err := s.Sync(context.Background(), c, TriggerManual)
	if err != nil {
		t.Fatal(err)
	}
	if res.Inserted != 0 || res.Rounded != 0 || len(st.roundings) != 3 {
		t.Errorf("deuxième passage : %+v et %d arrondis, attendu rien de nouveau et 3 arrondis", res, len(st.roundings))
	}
	// Fenêtre : dernière transaction connue (5 nov.) moins 7 jours = 29 oct.
	if got := b.gotFrom.Format("2006-01-02"); got != "2026-10-29" {
		t.Errorf("date_from = %s, attendu 2026-10-29", got)
	}
}

func TestSync_PlafondMensuelSurPlusieursTransactions(t *testing.T) {
	st := newFakeStore()
	st.settings = rounding.Settings{StepCents: 1000, Multiplier: 1, TxCapCents: 2000, MonthlyCapCents: 500}
	b := &fakeBank{items: []enablebanking.Item{
		item(t, "a", "BOOK", "2026-11-02", "7.00", "DBIT", "CARTE A"), // 3,00
		item(t, "b", "BOOK", "2026-11-03", "8.00", "DBIT", "CARTE B"), // 2,00 → cumul 5,00
		item(t, "c", "BOOK", "2026-11-04", "1.00", "DBIT", "CARTE C"), // 9,00 → 0 (plafond)
		item(t, "d", "BOOK", "2026-10-30", "9.00", "DBIT", "CARTE D"), // octobre : 1,00
	}}
	s, c := newService(st, b, "2026-11-08 06:30")
	res, err := s.Sync(context.Background(), c, TriggerScheduled)
	if err != nil {
		t.Fatal(err)
	}
	if res.SavedCents != 300+200+0+100 {
		t.Errorf("épargne %d c, attendu 600 c", res.SavedCents)
	}
	byTx := map[uuid.UUID]rounding.Result{}
	for _, r := range st.roundings {
		byTx[r.TransactionID] = r.Result
	}
	if got := byTx[st.ids["c"]]; got.FinalCents != 0 || got.LimitedBy != rounding.LimitMonthly {
		t.Errorf("transaction c : %+v, attendu 0 borné par le plafond mensuel", got)
	}
}

func TestSync_ErreursBanque(t *testing.T) {
	tests := []struct {
		err        error
		wantStatus string
		retry      bool
	}{
		{enablebanking.ErrConsent, "expired", false},
		{enablebanking.ErrCredentials, "error", false},
		{fmt.Errorf("%w : HTTP 502", enablebanking.ErrTemporary), "active", true},
		{enablebanking.ErrRateLimited, "active", false},
	}
	for _, tt := range tests {
		t.Run(tt.wantStatus+"/"+tt.err.Error(), func(t *testing.T) {
			st, b := newFakeStore(), &fakeBank{err: tt.err}
			s, c := newService(st, b, "2026-11-08 06:30")
			_, err := s.Sync(context.Background(), c, TriggerScheduled)
			if !errors.Is(err, tt.err) {
				t.Fatalf("erreur %v, attendu %v", err, tt.err)
			}
			if st.status != tt.wantStatus {
				t.Errorf("statut %q, attendu %q", st.status, tt.wantStatus)
			}
			if Retryable(err) != tt.retry {
				t.Errorf("Retryable = %v, attendu %v", Retryable(err), tt.retry)
			}
			if st.runs[0][:6] != "error:" || st.lastErr == "" {
				t.Errorf("exécution non notée en erreur : %v / %q", st.runs, st.lastErr)
			}
		})
	}
}

func TestSync_TransactionIllisibleNeModifieRien(t *testing.T) {
	st := newFakeStore()
	b := &fakeBank{items: []enablebanking.Item{
		item(t, "ok", "BOOK", "2026-11-02", "7.50", "DBIT", "CARTE A"),
		item(t, "ko", "BOOK", "2026-11-03", "7,50", "DBIT", "CARTE B"), // montant mal formé
	}}
	s, c := newService(st, b, "2026-11-08 06:30")
	_, err := s.Sync(context.Background(), c, TriggerScheduled)
	if !errors.Is(err, enablebanking.ErrBadResponse) {
		t.Fatalf("erreur %v, attendu ErrBadResponse", err)
	}
	if len(st.txs) != 0 || st.status != "error" {
		t.Errorf("%d transactions écrites, statut %q : attendu 0 et error (EF-13)", len(st.txs), st.status)
	}
}

func TestSync_PanneBaseAnnuleLeLot(t *testing.T) {
	st := newFakeStore()
	st.failInsert = true
	b := &fakeBank{items: []enablebanking.Item{
		item(t, "a", "BOOK", "2026-11-02", "7.50", "DBIT", "CARTE A"),
		item(t, "boom", "BOOK", "2026-11-03", "7.50", "DBIT", "CARTE B"),
	}}
	s, c := newService(st, b, "2026-11-08 06:30")
	if _, err := s.Sync(context.Background(), c, TriggerScheduled); err == nil {
		t.Fatal("erreur attendue")
	}
	if len(st.txs) != 0 || len(st.roundings) != 0 {
		t.Errorf("lot non annulé : %d transactions, %d arrondis", len(st.txs), len(st.roundings))
	}
}

func TestEligibility(t *testing.T) {
	act := at("2026-10-20 00:00")
	base := bank.Transaction{Debit: true, Currency: "EUR", AmountCents: 1234, BookedOn: at("2026-10-20 00:00"), IsCard: true}
	cases := map[string]func(*bank.Transaction){
		"":                     func(*bank.Transaction) {},
		ReasonCredit:           func(t *bank.Transaction) { t.Debit = false },
		ReasonForeignCurrency:  func(t *bank.Transaction) { t.Currency = "USD" },
		ReasonZeroAmount:       func(t *bank.Transaction) { t.AmountCents = 0 },
		ReasonBeforeActivation: func(t *bank.Transaction) { t.BookedOn = at("2026-10-19 00:00") },
		ReasonNotCard:          func(t *bank.Transaction) { t.IsCard = false },
	}
	for want, mut := range cases {
		tx := base
		mut(&tx)
		if got := Eligibility(tx, act, true); got != want {
			t.Errorf("attendu %q, obtenu %q", want, got)
		}
	}
	if got := Eligibility(base, act, false); got != ReasonUntrackedAccount {
		t.Errorf("compte non suivi : %q", got)
	}
}

func TestDayEtMonth(t *testing.T) {
	// 23:30 UTC le 31 octobre = 00:30 le 1er novembre à Paris.
	d := Day(time.Date(2026, 10, 31, 23, 30, 0, 0, time.UTC))
	if d.Format("2006-01-02") != "2026-11-01" {
		t.Errorf("Day = %s, attendu 2026-11-01", d.Format("2006-01-02"))
	}
	if m := Month(at("2026-11-17 00:00")); m.Format("2006-01-02") != "2026-11-01" {
		t.Errorf("Month = %s", m.Format("2006-01-02"))
	}
}
