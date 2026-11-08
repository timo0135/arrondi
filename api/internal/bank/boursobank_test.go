package bank

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// Extraits de la réponse réelle du J1 (7 novembre 2026), compte courant,
// noms de personnes remplacés.
const j1CompteCourant = `[
 {"entry_reference":"avis-56b5e22629280da6ccb10a194b9d7ab2","transaction_amount":{"currency":"EUR","amount":"24"},
  "credit_debit_indicator":"DBIT","status":"PDNG","booking_date":null,"transaction_date":"2026-11-06",
  "remittance_information":["100% COIFFURE\\BREST\\ FR"],"transaction_id":null},
 {"entry_reference":"32713209834","transaction_amount":{"currency":"EUR","amount":"200"},
  "credit_debit_indicator":"DBIT","status":"BOOK","booking_date":"2026-11-05","transaction_date":"2026-11-05",
  "remittance_information":["VIR INST DUPONT JEAN","Virement remboursement"],"transaction_id":null},
 {"entry_reference":"32710252754","transaction_amount":{"currency":"EUR","amount":"105.42"},
  "credit_debit_indicator":"DBIT","status":"BOOK","booking_date":"2026-11-05","transaction_date":"2026-11-05",
  "remittance_information":["PRLV SEPA MACIF Loir Bretagne Production","-PRELEV 1305112026"],"transaction_id":null},
 {"entry_reference":"32561334197","transaction_amount":{"currency":"EUR","amount":"1095.9"},
  "credit_debit_indicator":"CRDT","status":"BOOK","booking_date":"2026-10-28","transaction_date":"2026-10-28",
  "remittance_information":["VIR SEPA EMPLOYEUR"],"transaction_id":null},
 {"entry_reference":"32722526410","transaction_amount":{"currency":"EUR","amount":"69.37"},
  "credit_debit_indicator":"DBIT","status":"BOOK","booking_date":"2026-11-05","transaction_date":"2026-11-04",
  "remittance_information":["CARTE 04/11/26 TOTAL 4 CB*6804"],"transaction_id":null},
 {"entry_reference":"32684943033","transaction_amount":{"currency":"EUR","amount":"35.77"},
  "credit_debit_indicator":"DBIT","status":"BOOK","booking_date":"2026-11-03","transaction_date":"2026-11-02",
  "remittance_information":["CARTE 02/11/26 LECLERC LANDERNEA CB*6804"],"transaction_id":null},
 {"entry_reference":"32684943004","transaction_amount":{"currency":"EUR","amount":"58.99"},
  "credit_debit_indicator":"DBIT","status":"BOOK","booking_date":"2026-11-03","transaction_date":"2026-11-02",
  "remittance_information":["CARTE 02/11/26 E.LECLERC CB*6804"],"transaction_id":null}
]`

func loadJ1(t *testing.T) []RawTransaction {
	t.Helper()
	var txs []RawTransaction
	if err := json.Unmarshal([]byte(j1CompteCourant), &txs); err != nil {
		t.Fatal(err)
	}
	return txs
}

func TestBoursoBank_NormalizeJ1(t *testing.T) {
	p := BoursoBank{}
	type want struct {
		id     string
		cents  int64
		debit  bool
		isCard bool
		booked string
	}
	expected := []*want{
		nil, // PDNG : ignorée
		{"32713209834", 20000, true, false, "2026-11-05"},
		{"32710252754", 10542, true, false, "2026-11-05"},
		{"32561334197", 109590, false, false, "2026-10-28"},
		{"32722526410", 6937, true, true, "2026-11-05"},
		{"32684943033", 3577, true, true, "2026-11-03"},
		{"32684943004", 5899, true, true, "2026-11-03"},
	}
	for i, raw := range loadJ1(t) {
		got, err := Normalize(p, raw)
		w := expected[i]
		if w == nil {
			if !errors.Is(err, ErrSkip) {
				t.Errorf("tx %d : attendu ErrSkip, obtenu %v", i, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("tx %d : %v", i, err)
		}
		if got.ExternalID != w.id || got.AmountCents != w.cents || got.Debit != w.debit ||
			got.IsCard != w.isCard || got.BookedOn.Format("2006-01-02") != w.booked || got.Currency != "EUR" {
			t.Errorf("tx %d : %+v, attendu %+v", i, got, *w)
		}
		if got.BookedOn.Location().String() != "Europe/Paris" {
			t.Errorf("tx %d : fuseau %s, attendu Europe/Paris", i, got.BookedOn.Location())
		}
	}
}

func TestBoursoBank_Recognizes(t *testing.T) {
	p := BoursoBank{}
	for _, name := range []string{"Boursorama Banque", "boursorama banque", " Boursorama Banque "} {
		if !p.Recognizes(name) {
			t.Errorf("%q non reconnu", name)
		}
	}
	for _, name := range []string{"Crédit Agricole", "Boursorama", "La Banque Postale"} {
		if p.Recognizes(name) {
			t.Errorf("%q reconnu à tort", name)
		}
	}
	if got, ok := Find([]Profile{BoursoBank{}}, "Boursorama Banque"); !ok || got.Code() != "boursobank" {
		t.Error("Find ne trouve pas BoursoBank")
	}
	if _, ok := Find([]Profile{BoursoBank{}}, "Inconnue"); ok {
		t.Error("Find trouve une banque inconnue")
	}
}

func TestBoursoBank_SyncAccount(t *testing.T) {
	p := BoursoBank{}
	if !p.SyncAccount(RawAccount{CashAccountType: "CACC", Product: "CAV - BOURSOBANK"}) {
		t.Error("le compte courant doit être synchronisé")
	}
	if p.SyncAccount(RawAccount{CashAccountType: "CARD", Product: "Carte bancaire à débit immédiat"}) {
		t.Error("le compte carte ne doit pas être synchronisé (doublons)")
	}
}

func TestBoursoBank_StableID(t *testing.T) {
	p := BoursoBank{}
	id := "abc"
	if got, _ := p.StableID(RawTransaction{TransactionID: &id, EntryReference: "123"}); got != "abc" {
		t.Errorf("transaction_id prioritaire : obtenu %q", got)
	}
	if _, err := p.StableID(RawTransaction{}); err == nil {
		t.Error("sans référence : erreur attendue")
	}
	if _, err := p.StableID(RawTransaction{EntryReference: "avis-123"}); err == nil {
		t.Error("référence provisoire avis- : erreur attendue")
	}
}

func TestBoursoBank_MaxConsent(t *testing.T) {
	if got := (BoursoBank{}).MaxConsent(); got != 15552000*time.Second {
		t.Errorf("MaxConsent = %v, attendu 180 jours", got)
	}
}

func TestNormalize_Erreurs(t *testing.T) {
	p := BoursoBank{}
	date := "2026-11-05"
	bad := "05/11/2026"
	base := RawTransaction{EntryReference: "1", Status: "BOOK", BookingDate: &date, CreditDebitIndicator: "DBIT"}
	base.TransactionAmount.Amount = "1.00"

	noDate := base
	noDate.BookingDate = nil
	if _, err := Normalize(p, noDate); !errors.Is(err, ErrSkip) {
		t.Errorf("sans date : ErrSkip attendu, obtenu %v", err)
	}
	badDate := base
	badDate.BookingDate = &bad
	if _, err := Normalize(p, badDate); err == nil {
		t.Error("date mal formée : erreur attendue")
	}
	badSide := base
	badSide.CreditDebitIndicator = "XXX"
	if _, err := Normalize(p, badSide); err == nil {
		t.Error("sens inconnu : erreur attendue")
	}
}

func TestParseAmountCents(t *testing.T) {
	ok := map[string]int64{
		"24": 2400, "105.42": 10542, "1095.9": 109590, "0.01": 1, "0": 0,
		"9.67": 967, "236": 23600, " 14.1 ": 1410,
	}
	for in, want := range ok {
		got, err := ParseAmountCents(in)
		if err != nil || got != want {
			t.Errorf("ParseAmountCents(%q) = %d, %v ; attendu %d", in, got, err, want)
		}
	}
	for _, in := range []string{"", "-12.50", "+3", "1.234", "12.", ".5", "1,50", "abc", "1.2a", "99999999999999999999"} {
		if _, err := ParseAmountCents(in); err == nil {
			t.Errorf("ParseAmountCents(%q) : erreur attendue", in)
		}
	}
}
