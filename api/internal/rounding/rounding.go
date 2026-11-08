// Package rounding est le moteur de calcul des arrondis d'Arrondi
// (cahier des charges, section 4.3).
//
// Le package est pur : pas de base de données, pas de réseau, pas d'horloge.
// Tous les montants sont en centimes (int64), jamais en float.
//
// Formule, pour un montant m, un palier p et un multiplicateur k :
//
//	base  = (p − m mod p) mod p        // 0 si m est déjà un multiple de p (D-02)
//	brut  = base × k
//	final = min(brut, Ptx, R_mois)
//
// où R_mois = max(0, Pmois − arrondis finaux déjà enregistrés ce mois-ci),
// illimité quand il n'y a pas de plafond mensuel.
package rounding

import (
	"errors"
	"fmt"
)

// Settings regroupe les réglages d'arrondi d'un utilisateur (section 4.1).
type Settings struct {
	StepCents       int64 // palier p
	Multiplier      int64 // multiplicateur k
	TxCapCents      int64 // plafond par transaction Ptx
	MonthlyCapCents int64 // plafond mensuel Pmois ; 0 = aucun plafond
}

// DefaultSettings renvoie les valeurs par défaut du cahier des charges :
// palier 1 €, multiplicateur 1, plafond par transaction 20 €, pas de plafond mensuel.
func DefaultSettings() Settings {
	return Settings{StepCents: 100, Multiplier: 1, TxCapCents: 2000, MonthlyCapCents: 0}
}

// ErrInvalidSettings est renvoyée (enveloppée) par Validate ; l'API la traduit en 422.
var ErrInvalidSettings = errors.New("réglages d'arrondi invalides")

// Validate vérifie les plages permises (section 4.1).
func (s Settings) Validate() error {
	switch {
	case s.StepCents < 50 || s.StepCents > 5000 || s.StepCents%50 != 0:
		return fmt.Errorf("%w : palier %d c (attendu 0,50 € à 50,00 € par pas de 0,50 €)", ErrInvalidSettings, s.StepCents)
	case !validMultiplier(s.Multiplier):
		return fmt.Errorf("%w : multiplicateur %d (attendu 1, 2, 3, 5 ou 10)", ErrInvalidSettings, s.Multiplier)
	case s.TxCapCents < 100 || s.TxCapCents > 10000 || s.TxCapCents%100 != 0:
		return fmt.Errorf("%w : plafond par transaction %d c (attendu 1 € à 100 € par pas de 1 €)", ErrInvalidSettings, s.TxCapCents)
	case s.MonthlyCapCents != 0 && (s.MonthlyCapCents < 500 || s.MonthlyCapCents > 100000 || s.MonthlyCapCents%500 != 0):
		return fmt.Errorf("%w : plafond mensuel %d c (attendu aucun, ou 5 € à 1 000 € par pas de 5 €)", ErrInvalidSettings, s.MonthlyCapCents)
	}
	return nil
}

func validMultiplier(k int64) bool {
	switch k {
	case 1, 2, 3, 5, 10:
		return true
	}
	return false
}

// Limit indique ce qui a borné l'arrondi final, pour l'afficher dans l'historique.
type Limit string

const (
	LimitNone    Limit = ""        // final = brut
	LimitTxCap   Limit = "tx_cap"  // borné par Ptx
	LimitMonthly Limit = "monthly" // borné par R_mois
)

// Result détaille le calcul pour une transaction éligible.
type Result struct {
	BaseCents  int64 // arrondi de base
	GrossCents int64 // brut = base × k
	FinalCents int64 // montant mis de côté
	LimitedBy  Limit
}

// Compute calcule l'arrondi d'une transaction éligible de amountCents (> 0).
// monthToDateCents est la somme des arrondis finaux déjà enregistrés pour le
// mois de la transaction (transactions traitées dans l'ordre d'enregistrement).
// Les réglages doivent avoir été validés ; Compute panique sur un montant ≤ 0
// ou un palier ≤ 0, qui sont des erreurs de programmation de l'appelant.
func Compute(s Settings, amountCents, monthToDateCents int64) Result {
	if amountCents <= 0 {
		panic(fmt.Sprintf("rounding.Compute : montant %d ≤ 0 (transaction non éligible)", amountCents))
	}
	if s.StepCents <= 0 {
		panic("rounding.Compute : palier ≤ 0 (réglages non validés)")
	}

	base := (s.StepCents - amountCents%s.StepCents) % s.StepCents
	gross := base * s.Multiplier

	r := Result{BaseCents: base, GrossCents: gross, FinalCents: gross}

	if r.FinalCents > s.TxCapCents {
		r.FinalCents, r.LimitedBy = s.TxCapCents, LimitTxCap
	}
	if s.MonthlyCapCents > 0 {
		remaining := s.MonthlyCapCents - monthToDateCents
		if remaining < 0 {
			remaining = 0
		}
		if r.FinalCents > remaining {
			r.FinalCents, r.LimitedBy = remaining, LimitMonthly
		}
	}
	return r
}
