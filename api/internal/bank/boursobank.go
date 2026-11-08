package bank

import (
	"errors"
	"strings"
	"time"
)

// BoursoBank est le profil de BoursoBank, validé au J1 (7 novembre 2026)
// sur le compte de l'auteur via Enable Banking :
//
//   - nom ASPSP : "Boursorama Banque" ;
//   - la session expose le compte courant (CACC) ET la carte (CARD) ; chaque
//     paiement par carte apparaît dans les deux, avec des références différentes.
//     On ne synchronise que le compte courant pour ne pas compter deux fois ;
//   - un paiement par carte a un libellé qui commence par "CARTE "
//     (ex. "CARTE 04/11/26 TOTAL 4 CB*6804") ;
//   - transaction_id est toujours null, entry_reference est présent et unique ;
//   - consentement maximal : 15 552 000 s (180 jours).
type BoursoBank struct{}

func (BoursoBank) Code() string { return "boursobank" }

func (BoursoBank) Recognizes(aspspName string) bool {
	return strings.EqualFold(strings.TrimSpace(aspspName), "Boursorama Banque")
}

func (BoursoBank) SyncAccount(a RawAccount) bool {
	return a.CashAccountType == "CACC"
}

func (BoursoBank) IsCardPayment(t RawTransaction) bool {
	return len(t.RemittanceInfo) > 0 && strings.HasPrefix(t.RemittanceInfo[0], "CARTE ")
}

func (BoursoBank) StableID(t RawTransaction) (string, error) {
	if t.TransactionID != nil && *t.TransactionID != "" {
		return *t.TransactionID, nil
	}
	ref := strings.TrimSpace(t.EntryReference)
	if ref == "" {
		return "", errors.New("boursobank : transaction sans entry_reference ni transaction_id")
	}
	if strings.HasPrefix(ref, "avis-") {
		// Référence provisoire d'une opération en attente : elle change à la comptabilisation.
		return "", errors.New("boursobank : référence provisoire " + ref + " sur une transaction comptabilisée")
	}
	return ref, nil
}

func (BoursoBank) MaxConsent() time.Duration { return 180 * 24 * time.Hour }
