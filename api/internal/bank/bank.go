// Package bank définit le contrat commun aux banques prises en charge (D-17)
// et la conversion des transactions Enable Banking en transactions normalisées.
//
// Le reste de l'application ne connaît que l'interface Profile et le type
// Transaction : ajouter une banque = ajouter un fichier qui implémente Profile.
package bank

import (
	"errors"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata" // fuseaux embarqués : l'image distroless n'en a pas
)

// RawTransaction reprend les champs utiles d'une transaction renvoyée par
// GET /accounts/{uid}/transactions (Enable Banking). Les autres champs sont ignorés.
type RawTransaction struct {
	EntryReference       string   `json:"entry_reference"`
	TransactionID        *string  `json:"transaction_id"`
	CreditDebitIndicator string   `json:"credit_debit_indicator"` // "DBIT" ou "CRDT"
	Status               string   `json:"status"`                 // "BOOK", "PDNG", ...
	BookingDate          *string  `json:"booking_date"`           // "2026-11-05"
	TransactionDate      *string  `json:"transaction_date"`
	RemittanceInfo       []string `json:"remittance_information"`
	TransactionAmount    struct {
		Currency string `json:"currency"`
		Amount   string `json:"amount"` // "105.42", "24", "1095.9"
	} `json:"transaction_amount"`
}

// RawAccount reprend les champs utiles d'un compte de la session.
type RawAccount struct {
	UID             string `json:"uid"`
	Name            string `json:"name"`
	CashAccountType string `json:"cash_account_type"` // "CACC", "CARD", ...
	Product         string `json:"product"`
}

// Transaction est une transaction normalisée, indépendante de la banque.
type Transaction struct {
	ExternalID  string    // identifiant stable, unique par compte
	BookedOn    time.Time // date de comptabilisation, minuit Europe/Paris
	AmountCents int64     // toujours > 0 ; le sens est porté par Debit
	Debit       bool
	Currency    string
	Label       string // première ligne du libellé
	IsCard      bool   // paiement par carte selon la règle de la banque (D-06)
}

// Profile est le contrat d'une banque (cahier des charges, D-17).
type Profile interface {
	// Code est l'identifiant interne stable de la banque ("boursobank").
	Code() string
	// Recognizes dit si un nom d'ASPSP renvoyé par GET /aspsps est cette banque.
	Recognizes(aspspName string) bool
	// SyncAccount dit si un compte de la session doit être synchronisé.
	SyncAccount(a RawAccount) bool
	// IsCardPayment applique la règle de détection D-06 sur une transaction.
	IsCardPayment(t RawTransaction) bool
	// StableID renvoie l'identifiant stable d'une transaction comptabilisée.
	StableID(t RawTransaction) (string, error)
	// MaxConsent est la durée maximale de consentement acceptée par la banque.
	MaxConsent() time.Duration
}

// ErrSkip signale une transaction à ne pas importer (en attente, sans date…).
var ErrSkip = errors.New("transaction ignorée")

var paris = mustLoadParis()

func mustLoadParis() *time.Location {
	loc, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		panic("fuseau Europe/Paris introuvable : " + err.Error())
	}
	return loc
}

// Normalize convertit une transaction brute avec le profil de sa banque.
// Elle renvoie ErrSkip (enveloppée) pour les transactions qui ne doivent pas
// être importées : seules les transactions comptabilisées (BOOK) le sont,
// car la référence d'une opération en attente change quand elle est comptabilisée.
func Normalize(p Profile, t RawTransaction) (Transaction, error) {
	if t.Status != "BOOK" {
		return Transaction{}, fmt.Errorf("%w : statut %q", ErrSkip, t.Status)
	}
	if t.BookingDate == nil || *t.BookingDate == "" {
		return Transaction{}, fmt.Errorf("%w : pas de date de comptabilisation", ErrSkip)
	}
	booked, err := time.ParseInLocation("2006-01-02", *t.BookingDate, paris)
	if err != nil {
		return Transaction{}, fmt.Errorf("date de comptabilisation %q : %w", *t.BookingDate, err)
	}
	cents, err := ParseAmountCents(t.TransactionAmount.Amount)
	if err != nil {
		return Transaction{}, err
	}
	var debit bool
	switch t.CreditDebitIndicator {
	case "DBIT":
		debit = true
	case "CRDT":
		debit = false
	default:
		return Transaction{}, fmt.Errorf("sens inconnu %q", t.CreditDebitIndicator)
	}
	id, err := p.StableID(t)
	if err != nil {
		return Transaction{}, err
	}
	label := ""
	if len(t.RemittanceInfo) > 0 {
		label = strings.TrimSpace(t.RemittanceInfo[0])
	}
	return Transaction{
		ExternalID:  id,
		BookedOn:    booked,
		AmountCents: cents,
		Debit:       debit,
		Currency:    t.TransactionAmount.Currency,
		Label:       label,
		IsCard:      p.IsCardPayment(t),
	}, nil
}

// ParseAmountCents convertit un montant décimal positif ("105.42", "24", "1095.9")
// en centimes, sans passer par un float. Plus de deux décimales est une erreur.
func ParseAmountCents(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("montant vide")
	}
	if strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+") {
		return 0, fmt.Errorf("montant %q : signe inattendu (le sens est dans credit_debit_indicator)", s)
	}
	whole, frac, hasDot := strings.Cut(s, ".")
	if whole == "" || (hasDot && frac == "") || len(frac) > 2 {
		return 0, fmt.Errorf("montant %q mal formé", s)
	}
	for len(frac) < 2 {
		frac += "0"
	}
	var cents int64
	for _, c := range whole + frac {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("montant %q mal formé", s)
		}
		if cents > (1<<62)/10 {
			return 0, fmt.Errorf("montant %q trop grand", s)
		}
		cents = cents*10 + int64(c-'0')
	}
	return cents, nil
}

// Find renvoie le profil qui reconnaît un nom d'ASPSP, ou false.
func Find(profiles []Profile, aspspName string) (Profile, bool) {
	for _, p := range profiles {
		if p.Recognizes(aspspName) {
			return p, true
		}
	}
	return nil, false
}
