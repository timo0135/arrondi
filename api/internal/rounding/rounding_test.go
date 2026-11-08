package rounding

import (
	"errors"
	"testing"
)

// Exemples de référence du cahier des charges, section 4.4.
func TestCompute_ExemplesCahierDesCharges(t *testing.T) {
	tests := []struct {
		name                  string
		amount, step, k       int64
		monthCap, monthToDate int64
		base, gross, final    int64
		limit                 Limit
	}{
		{"cas simple", 250, 100, 1, 0, 0, 50, 50, 50, LimitNone},
		{"multiple exact du palier", 200, 100, 1, 0, 0, 0, 0, 0, LimitNone},
		{"multiple exact, multiplicateur sans effet", 1000, 1000, 5, 0, 0, 0, 0, 0, LimitNone},
		{"palier de 5 €", 1235, 500, 1, 0, 0, 265, 265, 265, LimitNone},
		{"multiplicateur 2", 1235, 1000, 2, 0, 0, 765, 1530, 1530, LimitNone},
		{"multiplicateur 3", 4710, 1000, 3, 0, 0, 290, 870, 870, LimitNone},
		{"juste sous le plafond par transaction", 810, 1000, 10, 0, 0, 190, 1900, 1900, LimitNone},
		{"plafond par transaction de 20 € atteint", 310, 1000, 10, 0, 0, 690, 6900, 2000, LimitTxCap},
		{"palier personnalisé", 320, 50, 1, 0, 0, 30, 30, 30, LimitNone},
		{"plus petit arrondi possible", 1999, 100, 1, 0, 0, 1, 1, 1, LimitNone},
		{"plus grand arrondi de base pour un palier de 1 €", 1, 100, 1, 0, 0, 99, 99, 99, LimitNone},
		{"plafond mensuel 50 €, déjà 48 € cumulés", 310, 100, 10, 5000, 4800, 90, 900, 200, LimitMonthly},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Settings{StepCents: tt.step, Multiplier: tt.k, TxCapCents: 2000, MonthlyCapCents: tt.monthCap}
			if err := s.Validate(); err != nil {
				t.Fatalf("réglages du test invalides : %v", err)
			}
			got := Compute(s, tt.amount, tt.monthToDate)
			want := Result{BaseCents: tt.base, GrossCents: tt.gross, FinalCents: tt.final, LimitedBy: tt.limit}
			if got != want {
				t.Errorf("Compute(%d) = %+v, attendu %+v", tt.amount, got, want)
			}
		})
	}
}

// Montants réels relevés au J1 sur le compte BoursoBank (réglages par défaut).
func TestCompute_TransactionsReellesJ1(t *testing.T) {
	tests := []struct {
		label        string
		amount, want int64
	}{
		{"TOTAL 4", 6937, 63},
		{"LECLERC LANDERNEA", 3577, 23},
		{"E.LECLERC", 5899, 1},
		{"UBER * EATS PEN", 967, 33},
		{"Leboncoin", 12839, 61},
		{"FNAC 03", 1410, 90},
		{"SAS LIBRAIRIE DIA", 2160, 40},
		{"PEUGEOT BREST 4", 23600, 0},
	}
	s := DefaultSettings()
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			if got := Compute(s, tt.amount, 0).FinalCents; got != tt.want {
				t.Errorf("arrondi de %d c = %d c, attendu %d c", tt.amount, got, tt.want)
			}
		})
	}
}

// Transactions d'un même mois traitées dans l'ordre : le plafond mensuel
// se remplit puis bloque les suivantes.
func TestCompute_SequenceMensuelle(t *testing.T) {
	s := Settings{StepCents: 100, Multiplier: 2, TxCapCents: 2000, MonthlyCapCents: 500}
	steps := []struct {
		amount, final int64
		limit         Limit
	}{
		{23600, 0, LimitNone},     // multiple exact
		{2160, 80, LimitNone},     // cumul 0,80 €
		{1410, 180, LimitNone},    // cumul 2,60 €
		{12839, 122, LimitNone},   // cumul 3,82 €
		{3127, 118, LimitMonthly}, // brut 1,46 €, reste 1,18 € → cumul 5,00 €
		{967, 0, LimitMonthly},    // plafond atteint
	}
	var monthToDate int64
	for i, st := range steps {
		got := Compute(s, st.amount, monthToDate)
		if got.FinalCents != st.final || got.LimitedBy != st.limit {
			t.Fatalf("étape %d (%d c) : final %d c / %q, attendu %d c / %q",
				i+1, st.amount, got.FinalCents, got.LimitedBy, st.final, st.limit)
		}
		monthToDate += got.FinalCents
	}
	if monthToDate != 500 {
		t.Errorf("cumul du mois = %d c, attendu 500 c", monthToDate)
	}
}

// Cumul déjà au-delà du plafond (plafond abaissé en cours de mois) : R_mois borné à 0.
func TestCompute_CumulAuDelaDuPlafond(t *testing.T) {
	s := Settings{StepCents: 100, Multiplier: 1, TxCapCents: 2000, MonthlyCapCents: 500}
	got := Compute(s, 250, 900)
	if got.FinalCents != 0 || got.LimitedBy != LimitMonthly {
		t.Errorf("got %+v, attendu final 0 borné par le plafond mensuel", got)
	}
}

func TestValidate(t *testing.T) {
	ok := []Settings{
		DefaultSettings(),
		{StepCents: 50, Multiplier: 10, TxCapCents: 100, MonthlyCapCents: 500},
		{StepCents: 5000, Multiplier: 3, TxCapCents: 10000, MonthlyCapCents: 100000},
		{StepCents: 250, Multiplier: 5, TxCapCents: 1500, MonthlyCapCents: 0},
	}
	for _, s := range ok {
		if err := s.Validate(); err != nil {
			t.Errorf("%+v refusé à tort : %v", s, err)
		}
	}

	ko := map[string]Settings{
		"palier trop petit":          {StepCents: 0, Multiplier: 1, TxCapCents: 2000},
		"palier hors pas de 0,50 €":  {StepCents: 120, Multiplier: 1, TxCapCents: 2000},
		"palier trop grand":          {StepCents: 5050, Multiplier: 1, TxCapCents: 2000},
		"multiplicateur 4":           {StepCents: 100, Multiplier: 4, TxCapCents: 2000},
		"plafond tx à 0,50 €":        {StepCents: 100, Multiplier: 1, TxCapCents: 50},
		"plafond tx trop grand":      {StepCents: 100, Multiplier: 1, TxCapCents: 10100},
		"plafond tx hors pas de 1 €": {StepCents: 100, Multiplier: 1, TxCapCents: 1550},
		"plafond mensuel 2 €":        {StepCents: 100, Multiplier: 1, TxCapCents: 2000, MonthlyCapCents: 200},
		"plafond mensuel hors pas":   {StepCents: 100, Multiplier: 1, TxCapCents: 2000, MonthlyCapCents: 1200},
		"plafond mensuel négatif":    {StepCents: 100, Multiplier: 1, TxCapCents: 2000, MonthlyCapCents: -500},
	}
	for name, s := range ko {
		if err := s.Validate(); !errors.Is(err, ErrInvalidSettings) {
			t.Errorf("%s : %+v accepté à tort (err = %v)", name, s, err)
		}
	}
}

func TestCompute_PaniqueSurMontantNonEligible(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Compute(0) aurait dû paniquer")
		}
	}()
	Compute(DefaultSettings(), 0, 0)
}
