package httpapi

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
	"unicode"

	"github.com/timotheebenchergui/arrondi/api/internal/rounding"
	"github.com/timotheebenchergui/arrondi/api/internal/store"
)

// Routes de la version ordinateur (D-20 à D-23, EF-38 à EF-44). Tous les
// calculs sont faits ici, en SQL ou avec le moteur d'arrondis (D-21).

type MerchantOut struct {
	Merchant   string `json:"merchant"`
	Count      int32  `json:"count"`
	SavedCents int64  `json:"saved_cents"`
}

type MonthStatsOut struct {
	Month                     string           `json:"month" doc:"AAAA-MM"`
	TransactionsCount         int32            `json:"transactions_count" doc:"Toutes les opérations du mois"`
	CardPaymentsCount         int32            `json:"card_payments_count" doc:"Paiements carte éligibles"`
	RoundedCount              int32            `json:"rounded_count" doc:"Paiements dont l'arrondi gardé est supérieur à 0"`
	ExcludedCount             int32            `json:"excluded_count"`
	ExcludedByReason          map[string]int32 `json:"excluded_by_reason" doc:"Nombre d'opérations exclues par raison (section 4.2)"`
	CappedCount               int32            `json:"capped_count" doc:"Arrondis limités par un plafond"`
	AverageRoundingCents      int64            `json:"average_rounding_cents" doc:"Arrondi gardé moyen des paiements arrondis"`
	CardSpendingCents         int64            `json:"card_spending_cents" doc:"Somme des paiements carte éligibles"`
	SavedCents                int64            `json:"saved_cents"`
	PreviousMonthSameDayCents int64            `json:"previous_month_same_day_cents" doc:"Mois en cours : épargne du mois précédent jusqu'au même jour ; mois passé : épargne de tout le mois précédent"`
	TopMerchants              []MerchantOut    `json:"top_merchants" doc:"5 commerçants au plus fort arrondi cumulé (D-23)"`
}

type PreviewExample struct {
	AmountCents     int64   `json:"amount_cents"`
	BaseCents       int64   `json:"base_cents"`
	MultipliedCents int64   `json:"multiplied_cents"`
	FinalCents      int64   `json:"final_cents" doc:"Sans tenir compte du plafond mensuel"`
	LimitedBy       *string `json:"limited_by" doc:"tx_cap ou null"`
}

type PreviewLastMonth struct {
	Month             string `json:"month" doc:"AAAA-MM, dernier mois complet"`
	SavedCents        int64  `json:"saved_cents" doc:"Ce que ces réglages auraient donné"`
	ActualSavedCents  int64  `json:"actual_saved_cents" doc:"Ce qui a vraiment été épargné ce mois-là"`
	CardPaymentsCount int32  `json:"card_payments_count"`
}

type PreviewOut struct {
	Example   PreviewExample   `json:"example"`
	LastMonth PreviewLastMonth `json:"last_month"`
}

type SyncRunOut struct {
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	Trigger    string     `json:"trigger" enum:"scheduled,manual"`
	Status     string     `json:"status" enum:"running,success,error"`
	Fetched    int32      `json:"fetched"`
	Inserted   int32      `json:"inserted"`
	ErrorCode  *string    `json:"error_code" doc:"Code d'erreur sans donnée personnelle ; null si la synchro a réussi"`
}

func transactionOut(r store.GetTransactionForUserRow) TransactionOut {
	t := TransactionOut{ID: r.ID, BookedOn: r.BookedOn.Format(dateFmt), AmountCents: r.AmountCents,
		Direction: r.Direction, Currency: r.Currency, Label: r.Label, Merchant: r.Merchant,
		IsEligible: r.IsEligible, ExclusionReason: r.ExclusionReason}
	if r.FinalCents != nil {
		t.Rounding = &RoundingOut{StepCents: *r.StepCents, Multiplier: *r.Multiplier, BaseCents: *r.BaseCents,
			GrossCents: *r.GrossCents, FinalCents: *r.FinalCents, LimitedBy: r.LimitedBy}
	}
	return t
}

// likePattern met q sous la forme comparée en SQL : minuscules, sans accents,
// caractères spéciaux de LIKE échappés (le \ est l'échappement par défaut).
func likePattern(q string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	plain, _, err := transform.String(t, strings.ToLower(q))
	if err != nil {
		plain = strings.ToLower(q)
	}
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(plain)
}

func parseMonth(v string, now time.Time) (time.Time, error) {
	if v == "" {
		return firstOfMonth(today(now)), nil
	}
	m, err := time.Parse("2006-01", v)
	if err != nil {
		return time.Time{}, huma.Error422UnprocessableEntity("month doit être au format AAAA-MM")
	}
	return m, nil
}

// goalProjection applique EF-41 : rythme = moyenne des 3 derniers mois complets
// depuis l'activation ; date estimée = mois où le cumul atteint la cible.
func (s *Server) goalProjection(ctx context.Context, userID uuid.UUID, savedCents, targetCents int64, now time.Time) (*int64, *string, error) {
	current := firstOfMonth(today(now))
	if savedCents >= targetCents {
		m := current.Format("2006-01")
		return nil, &m, nil
	}
	c, err := s.App.Q.GetConnectionByUser(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if c.ActivatedAt == nil {
		return nil, nil, nil
	}
	// Un mois est complet s'il est entièrement postérieur à l'activation et terminé.
	firstFull := firstOfMonth(today(*c.ActivatedAt)).AddDate(0, 1, 0)
	from := current.AddDate(0, -3, 0)
	if from.Before(firstFull) {
		from = firstFull
	}
	if !from.Before(current) {
		return nil, nil, nil
	}
	rows, err := s.App.Q.SavedByMonthBetween(ctx, store.SavedByMonthBetweenParams{UserID: userID, FromMonth: from, ToMonth: current})
	if err != nil {
		return nil, nil, err
	}
	months := 0
	for m := from; m.Before(current); m = m.AddDate(0, 1, 0) {
		months++
	}
	var sum int64
	for _, r := range rows {
		sum += r.SavedCents
	}
	pace := sum / int64(months)
	if pace <= 0 {
		return &pace, nil, nil
	}
	remaining := targetCents - savedCents
	need := (remaining + pace - 1) / pace // mois entiers, arrondis au-dessus
	est := current.AddDate(0, int(need), 0).Format("2006-01")
	return &pace, &est, nil
}

func (s *Server) goalOut(ctx context.Context, g store.Goal) (GoalOut, error) {
	out := GoalOut{GoalBody: goalBody(g)}
	t, err := s.App.Q.SavingsTotals(ctx, store.SavingsTotalsParams{UserID: g.UserID, Month: firstOfMonth(today(s.now()))})
	if err != nil {
		return out, err
	}
	out.SavedCents = t.TotalSavedCents
	out.MonthlyPaceCents, out.EstimatedMonth, err = s.goalProjection(ctx, g.UserID, t.TotalSavedCents, g.TargetCents, s.now())
	return out, err
}

// euros formate des centimes pour le CSV : "47,83", "-120,00".
func euros(c int64) string {
	sign := ""
	if c < 0 {
		sign, c = "-", -c
	}
	return fmt.Sprintf("%s%d,%02d", sign, c/100, c%100)
}

var exclusionFR = map[string]string{
	"credit": "crédit", "foreign_currency": "devise étrangère", "zero_amount": "montant nul",
	"before_activation": "avant l'activation", "not_card": "pas un paiement carte",
	"cash_withdrawal": "retrait d'espèces", "untracked_account": "compte non suivi",
}

func (s *Server) registerStats(api huma.API) {
	tag := []string{"Épargne"}

	type monthIn struct {
		Month string `query:"month" doc:"AAAA-MM ; mois en cours par défaut"`
	}
	huma.Register(api, huma.Operation{OperationID: "get-month-stats", Method: http.MethodGet, Path: "/stats/month",
		Summary: "Chiffres d'un mois (EF-38)", Tags: tag},
		func(ctx context.Context, in *monthIn) (*struct{ Body MonthStatsOut }, error) {
			u := userFrom(ctx)
			now := s.now()
			month, err := parseMonth(in.Month, now)
			if err != nil {
				return nil, err
			}
			q := s.App.Q
			st, err := q.MonthStats(ctx, store.MonthStatsParams{UserID: u.ID, Month: month})
			if err != nil {
				return nil, internalError(s.Log, "month_stats", err)
			}
			out := MonthStatsOut{Month: month.Format("2006-01"), TransactionsCount: st.TransactionsCount,
				CardPaymentsCount: st.CardPaymentsCount, RoundedCount: st.RoundedCount, ExcludedCount: st.ExcludedCount,
				CappedCount: st.CappedCount, CardSpendingCents: st.CardSpendingCents, SavedCents: st.SavedCents,
				ExcludedByReason: map[string]int32{}, TopMerchants: []MerchantOut{}}
			if st.RoundedCount > 0 {
				out.AverageRoundingCents = (st.SavedCents + int64(st.RoundedCount)/2) / int64(st.RoundedCount)
			}
			ex, err := q.MonthExclusions(ctx, store.MonthExclusionsParams{UserID: u.ID, Month: month})
			if err != nil {
				return nil, internalError(s.Log, "month_exclusions", err)
			}
			for _, e := range ex {
				out.ExcludedByReason[e.Reason] = e.Count
			}
			top, err := q.TopMerchants(ctx, store.TopMerchantsParams{UserID: u.ID, Month: month, PageSize: 5})
			if err != nil {
				return nil, internalError(s.Log, "top_merchants", err)
			}
			for _, m := range top {
				out.TopMerchants = append(out.TopMerchants, MerchantOut{Merchant: m.Merchant, Count: m.Count, SavedCents: m.SavedCents})
			}
			// Comparaison avec le mois précédent, au même jour pour le mois en cours.
			prev := month.AddDate(0, -1, 0)
			prevEnd := month.AddDate(0, 0, -1)
			if day := today(now); month.Equal(firstOfMonth(day)) {
				d := day.Day()
				if last := prevEnd.Day(); d > last {
					d = last
				}
				prevEnd = time.Date(prev.Year(), prev.Month(), d, 0, 0, 0, 0, time.UTC)
			}
			if out.PreviousMonthSameDayCents, err = q.SavedBookedBetween(ctx, store.SavedBookedBetweenParams{UserID: u.ID, FromDay: prev, ToDay: prevEnd}); err != nil {
				return nil, internalError(s.Log, "previous_month", err)
			}
			return &struct{ Body MonthStatsOut }{out}, nil
		})

	type idIn struct {
		ID uuid.UUID `path:"id"`
	}
	huma.Register(api, huma.Operation{OperationID: "get-transaction", Method: http.MethodGet, Path: "/transactions/{id}",
		Summary: "Une transaction et le détail de son arrondi (EF-40)", Tags: tag},
		func(ctx context.Context, in *idIn) (*struct{ Body TransactionOut }, error) {
			r, err := s.App.Q.GetTransactionForUser(ctx, store.GetTransactionForUserParams{UserID: userFrom(ctx).ID, ID: in.ID})
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, huma.Error404NotFound("transaction introuvable")
			}
			if err != nil {
				return nil, internalError(s.Log, "get_transaction", err)
			}
			return &struct{ Body TransactionOut }{transactionOut(r)}, nil
		})

	type exportIn struct {
		Month string `query:"month" required:"true" doc:"AAAA-MM"`
	}
	type csvOut struct {
		ContentType        string `header:"Content-Type"`
		ContentDisposition string `header:"Content-Disposition"`
		Body               []byte
	}
	huma.Register(api, huma.Operation{OperationID: "export-month-csv", Method: http.MethodGet, Path: "/transactions/export",
		Summary: "Exporter les opérations d'un mois en CSV (EF-44)", Tags: tag},
		func(ctx context.Context, in *exportIn) (*csvOut, error) {
			month, err := parseMonth(in.Month, s.now())
			if err != nil || in.Month == "" {
				return nil, huma.Error422UnprocessableEntity("month doit être au format AAAA-MM")
			}
			rows, err := s.App.Q.ListMonthTransactions(ctx, store.ListMonthTransactionsParams{UserID: userFrom(ctx).ID, Month: month})
			if err != nil {
				return nil, internalError(s.Log, "export_month", err)
			}
			var buf bytes.Buffer
			buf.WriteString("\xEF\xBB\xBF") // BOM : Excel lit l'UTF-8 sans casser les accents
			w := csv.NewWriter(&buf)
			w.Comma = ';'
			w.UseCRLF = true
			_ = w.Write([]string{"Date", "Libellé", "Commerçant", "Montant (€)", "Arrondi (€)", "Raison d'exclusion"})
			for _, r := range rows {
				amount := r.AmountCents
				if r.Direction == "debit" {
					amount = -amount
				}
				rnd, reason := "", ""
				if r.FinalCents != nil {
					rnd = euros(*r.FinalCents)
				}
				if r.ExclusionReason != nil {
					reason = exclusionFR[*r.ExclusionReason]
				}
				_ = w.Write([]string{r.BookedOn.Format("02/01/2006"), r.Label, r.Merchant, euros(amount), rnd, reason})
			}
			w.Flush()
			name := fmt.Sprintf(`attachment; filename="arrondi-%s.csv"`, month.Format("2006-01"))
			return &csvOut{ContentType: "text/csv; charset=utf-8", ContentDisposition: name, Body: buf.Bytes()}, nil
		})

	type previewIn struct {
		StepCents     int64 `query:"step_cents" required:"true"`
		Multiplier    int64 `query:"multiplier" required:"true"`
		TxCapCents    int64 `query:"tx_cap_cents" required:"true"`
		MonthCapCents int64 `query:"month_cap_cents" doc:"0 ou absent = aucun plafond mensuel"`
		AmountCents   int64 `query:"amount_cents" default:"2340" minimum:"1" maximum:"10000000" doc:"Montant d'essai"`
	}
	huma.Register(api, huma.Operation{OperationID: "preview-settings", Method: http.MethodGet, Path: "/settings/preview",
		Summary: "Aperçu de réglages, sans les enregistrer (EF-42)", Tags: []string{"Réglages"}},
		func(ctx context.Context, in *previewIn) (*struct{ Body PreviewOut }, error) {
			u := userFrom(ctx)
			rs := rounding.Settings{StepCents: in.StepCents, Multiplier: in.Multiplier, TxCapCents: in.TxCapCents, MonthlyCapCents: in.MonthCapCents}
			if err := rs.Validate(); err != nil {
				return nil, huma.Error422UnprocessableEntity(err.Error())
			}
			out := PreviewOut{}
			ex := rounding.Compute(rounding.Settings{StepCents: rs.StepCents, Multiplier: rs.Multiplier, TxCapCents: rs.TxCapCents}, in.AmountCents, 0)
			out.Example = PreviewExample{AmountCents: in.AmountCents, BaseCents: ex.BaseCents, MultipliedCents: ex.GrossCents, FinalCents: ex.FinalCents}
			if ex.LimitedBy != rounding.LimitNone {
				l := string(ex.LimitedBy)
				out.Example.LimitedBy = &l
			}
			last := firstOfMonth(today(s.now())).AddDate(0, -1, 0)
			amounts, err := s.App.Q.EligibleAmountsInMonth(ctx, store.EligibleAmountsInMonthParams{UserID: u.ID, Month: last})
			if err != nil {
				return nil, internalError(s.Log, "preview_amounts", err)
			}
			var mtd int64
			for _, a := range amounts {
				if a <= 0 {
					continue
				}
				mtd += rounding.Compute(rs, a, mtd).FinalCents
			}
			actual, err := s.App.Q.SavingsTotals(ctx, store.SavingsTotalsParams{UserID: u.ID, Month: last})
			if err != nil {
				return nil, internalError(s.Log, "preview_actual", err)
			}
			out.LastMonth = PreviewLastMonth{Month: last.Format("2006-01"), SavedCents: mtd,
				ActualSavedCents: actual.MonthSavedCents, CardPaymentsCount: int32(len(amounts))}
			return &struct{ Body PreviewOut }{out}, nil
		})

	type runsIn struct {
		Limit int32 `query:"limit" default:"10" minimum:"1" maximum:"50"`
	}
	huma.Register(api, huma.Operation{OperationID: "list-sync-runs", Method: http.MethodGet, Path: "/bank/sync-runs",
		Summary: "Dernières synchronisations (EF-43)", Tags: []string{"Banque"}},
		func(ctx context.Context, in *runsIn) (*struct{ Body []SyncRunOut }, error) {
			c, err := s.connection(ctx)
			if err != nil {
				return nil, err
			}
			rows, err := s.App.Q.ListSyncRuns(ctx, store.ListSyncRunsParams{ConnectionID: c.ID, PageSize: in.Limit})
			if err != nil {
				return nil, internalError(s.Log, "sync_runs", err)
			}
			out := make([]SyncRunOut, 0, len(rows))
			for _, r := range rows {
				out = append(out, SyncRunOut{StartedAt: r.StartedAt.UTC(), FinishedAt: utc(r.FinishedAt), Trigger: r.Trigger,
					Status: r.Status, Fetched: r.FetchedCount, Inserted: r.InsertedCount, ErrorCode: r.Error})
			}
			return &struct{ Body []SyncRunOut }{out}, nil
		})
}
