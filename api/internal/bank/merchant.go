package bank

import (
	"regexp"
	"strings"
	"unicode"
)

// MerchantNamer est implémentée par les profils qui savent extraire le nom du
// commerçant de leurs libellés (D-23). Sans elle, le libellé entier est utilisé.
type MerchantNamer interface {
	Merchant(label string) string
}

// MerchantOf renvoie le nom du commerçant d'un libellé, avec la règle du profil
// s'il en a une, puis en adoucissant les majuscules (comme initcap de PostgreSQL).
func MerchantOf(p Profile, label string) string {
	m := strings.TrimSpace(label)
	if mn, ok := p.(MerchantNamer); ok {
		if v := strings.TrimSpace(mn.Merchant(m)); v != "" {
			m = v
		}
	}
	return Initcap(m)
}

// Initcap met en majuscule la première lettre de chaque mot et le reste en
// minuscules ; un mot commence après tout caractère qui n'est ni lettre ni chiffre.
// C'est la règle de initcap() de PostgreSQL, utilisée par la migration 00002.
func Initcap(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevAlnum := false
	for _, r := range s {
		if prevAlnum {
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(unicode.ToUpper(r))
		}
		prevAlnum = unicode.IsLetter(r) || unicode.IsDigit(r)
	}
	return b.String()
}

// Libellés BoursoBank : "CARTE 04/11/26 TOTAL 4 CB*6804", "PRLV SEPA MACIF".
var (
	boursoCard   = regexp.MustCompile(`^CARTE [0-9]{2}/[0-9]{2}(/[0-9]{2})? +`)
	boursoCardNo = regexp.MustCompile(` +CB\*[0-9]+$`)
	boursoPrlv   = regexp.MustCompile(`^PRLV SEPA +`)
)

// Merchant retire le préfixe de date, le numéro de carte et le préfixe de prélèvement.
// La migration 00002 applique la même règle, en SQL, aux transactions déjà importées.
func (BoursoBank) Merchant(label string) string {
	m := boursoCard.ReplaceAllString(label, "")
	m = boursoCardNo.ReplaceAllString(m, "")
	m = boursoPrlv.ReplaceAllString(m, "")
	return m
}
