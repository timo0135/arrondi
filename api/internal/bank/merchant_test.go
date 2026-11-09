package bank

import "testing"

func TestMerchantOf_BoursoBank(t *testing.T) {
	tests := []struct{ label, want string }{
		{"CARTE 04/11/26 TOTAL 4 CB*6804", "Total 4"},
		{"CARTE 08/11 SUPERMARCHE CENTRE", "Supermarche Centre"},
		{"CARTE 05/11/26 AMAZON PAYMENTS CB*4417", "Amazon Payments"},
		{"PRLV SEPA MACIF", "Macif"},
		{"VIR SEPA M. DUPONT", "Vir Sepa M. Dupont"},
		{"  CARTE 01/01/26 L'ATELIER-CAFE CB*1  ", "L'Atelier-Cafe"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := MerchantOf(BoursoBank{}, tt.label); got != tt.want {
			t.Errorf("MerchantOf(%q) = %q, attendu %q", tt.label, got, tt.want)
		}
	}
}

func TestInitcap(t *testing.T) {
	for in, want := range map[string]string{"ÉPICERIE DU COIN": "Épicerie Du Coin", "abc2def": "Abc2def", "a.b": "A.B"} {
		if got := Initcap(in); got != want {
			t.Errorf("Initcap(%q) = %q, attendu %q", in, got, want)
		}
	}
}
