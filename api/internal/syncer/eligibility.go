package syncer

import (
	"time"

	"github.com/timotheebenchergui/arrondi/api/internal/bank"
)

// Codes d'exclusion (cahier des charges, section 4.2), identiques à la contrainte SQL.
const (
	ReasonCredit           = "credit"
	ReasonForeignCurrency  = "foreign_currency"
	ReasonZeroAmount       = "zero_amount"
	ReasonBeforeActivation = "before_activation"
	ReasonNotCard          = "not_card"
	ReasonCashWithdrawal   = "cash_withdrawal"
	ReasonUntrackedAccount = "untracked_account"
)

// Eligibility renvoie "" si la transaction est éligible, sinon le premier code
// d'exclusion rencontré, dans l'ordre de la section 4.2. Le statut comptabilisé
// est déjà garanti par bank.Normalize.
//
// activatedOn est le jour d'activation (minuit Europe/Paris) ; une transaction
// comptabilisée ce jour-là est éligible.
func Eligibility(t bank.Transaction, activatedOn time.Time, tracked bool) string {
	switch {
	case !t.Debit:
		return ReasonCredit
	case t.Currency != "EUR":
		return ReasonForeignCurrency
	case t.AmountCents == 0:
		return ReasonZeroAmount
	case t.BookedOn.Before(activatedOn):
		return ReasonBeforeActivation
	case !t.IsCard:
		// Les retraits d'espèces BoursoBank ne commencent pas par "CARTE " :
		// ils tombent ici. cash_withdrawal servira aux banques qui les distinguent.
		return ReasonNotCard
	case !tracked:
		return ReasonUntrackedAccount
	}
	return ""
}
