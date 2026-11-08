package httpapi

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/timotheebenchergui/arrondi/api/internal/rounding"
	"github.com/timotheebenchergui/arrondi/api/internal/store"
)

// --- types de sortie ---

type DashboardGoal struct {
	Name            string  `json:"name"`
	TargetCents     int64   `json:"target_cents"`
	ProgressPercent int     `json:"progress_percent"`
	TargetDate      *string `json:"target_date"`
}

type DashboardBank struct {
	Status        string     `json:"status" enum:"pending,active,expiring_soon,expired,error"`
	ExpiresInDays *int       `json:"expires_in_days"`
	LastSyncOkAt  *time.Time `json:"last_sync_ok_at"`
}

type DashboardOut struct {
	TotalSavedCents  int64          `json:"total_saved_cents"`
	MonthSavedCents  int64          `json:"month_saved_cents"`
	TransferredCents int64          `json:"transferred_cents"`
	ToTransferCents  int64          `json:"to_transfer_cents" doc:"Épargné moins viré, jamais négatif"`
	Month            string         `json:"month" doc:"AAAA-MM"`
	Goal             *DashboardGoal `json:"goal"`
	Bank             *DashboardBank `json:"bank"`
}

type MonthOut struct {
	Month             string `json:"month" doc:"AAAA-MM"`
	SavedCents        int64  `json:"saved_cents"`
	TransferredCents  int64  `json:"transferred_cents"`
	TransactionsCount int32  `json:"transactions_count"`
}

type RoundingOut struct {
	BaseCents  int32   `json:"base_cents"`
	GrossCents int64   `json:"gross_cents"`
	FinalCents int64   `json:"final_cents"`
	LimitedBy  *string `json:"limited_by" doc:"tx_cap, monthly ou null"`
}

type TransactionOut struct {
	ID              uuid.UUID    `json:"id"`
	BookedOn        string       `json:"booked_on"`
	AmountCents     int64        `json:"amount_cents"`
	Direction       string       `json:"direction" enum:"debit,credit"`
	Currency        string       `json:"currency"`
	Label           string       `json:"label"`
	IsEligible      bool         `json:"is_eligible"`
	ExclusionReason *string      `json:"exclusion_reason"`
	Rounding        *RoundingOut `json:"rounding"`
}

type PageOut[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"next_cursor" doc:"À repasser dans cursor pour la page suivante ; null s'il n'y en a pas"`
}

type SettingsBody struct {
	StepCents     int64  `json:"step_cents" doc:"Palier : 50 à 5000, multiple de 50"`
	Multiplier    int64  `json:"multiplier" enum:"1,2,3,5,10"`
	TxCapCents    int64  `json:"tx_cap_cents" doc:"Plafond par transaction : 100 à 10000, multiple de 100"`
	MonthCapCents *int64 `json:"month_cap_cents" required:"false" nullable:"true" doc:"null = aucun plafond ; sinon 500 à 100000, multiple de 500"`
}

type GoalBody struct {
	Name        string  `json:"name" minLength:"1" maxLength:"60"`
	TargetCents int64   `json:"target_cents"`
	TargetDate  *string `json:"target_date" required:"false" nullable:"true" format:"date"`
}

type TransferOut struct {
	ID            uuid.UUID `json:"id"`
	AmountCents   int64     `json:"amount_cents"`
	TransferredOn string    `json:"transferred_on"`
	CreatedAt     time.Time `json:"created_at"`
}

func settingsBody(st store.UserSetting) SettingsBody {
	s := store.SettingsFromRow(st)
	b := SettingsBody{StepCents: s.StepCents, Multiplier: s.Multiplier, TxCapCents: s.TxCapCents}
	if s.MonthlyCapCents > 0 {
		v := s.MonthlyCapCents
		b.MonthCapCents = &v
	}
	return b
}

func goalBody(g store.Goal) GoalBody {
	b := GoalBody{Name: g.Name, TargetCents: g.TargetCents}
	if g.TargetDate != nil {
		d := g.TargetDate.Format(dateFmt)
		b.TargetDate = &d
	}
	return b
}

func transferOut(t store.Transfer) TransferOut {
	return TransferOut{ID: t.ID, AmountCents: t.AmountCents, TransferredOn: t.TransferredOn.Format(dateFmt), CreatedAt: t.CreatedAt.UTC()}
}

// --- curseurs : base64url("AAAA-MM-JJ|uuid") ---

func encodeCursor(day time.Time, id uuid.UUID) *string {
	c := base64.RawURLEncoding.EncodeToString([]byte(day.Format(dateFmt) + "|" + id.String()))
	return &c
}

func decodeCursor(c string) (*time.Time, *uuid.UUID, error) {
	if c == "" {
		return nil, nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return nil, nil, huma.Error400BadRequest("cursor invalide")
	}
	ds, is, ok := strings.Cut(string(raw), "|")
	d, err1 := time.Parse(dateFmt, ds)
	id, err2 := uuid.Parse(is)
	if !ok || err1 != nil || err2 != nil {
		return nil, nil, huma.Error400BadRequest("cursor invalide")
	}
	return &d, &id, nil
}

// bankState déduit l'état affiché, dont « expire bientôt » (14 jours, section 9.5).
func bankState(c store.BankConnection, now time.Time) DashboardBank {
	b := DashboardBank{Status: c.Status, LastSyncOkAt: c.LastSyncOkAt}
	if c.ConsentValidUntil != nil {
		days := int(c.ConsentValidUntil.Sub(now).Hours() / 24)
		if days < 0 {
			days = 0
		}
		b.ExpiresInDays = &days
		if c.Status == "active" && days <= 14 {
			b.Status = "expiring_soon"
		}
	}
	return b
}

func (s *Server) registerSavings(api huma.API) {
	tag := []string{"Épargne"}

	huma.Register(api, huma.Operation{OperationID: "get-dashboard", Method: http.MethodGet, Path: "/dashboard", Summary: "Tableau de bord", Tags: tag},
		func(ctx context.Context, _ *struct{}) (*struct{ Body DashboardOut }, error) {
			u := userFrom(ctx)
			now := s.now()
			month := firstOfMonth(today(now))
			t, err := s.App.Q.SavingsTotals(ctx, store.SavingsTotalsParams{UserID: u.ID, Month: month})
			if err != nil {
				return nil, internalError(s.Log, "dashboard_totals", err)
			}
			out := DashboardOut{
				TotalSavedCents: t.TotalSavedCents, MonthSavedCents: t.MonthSavedCents,
				TransferredCents: t.TotalTransferredCents, Month: month.Format("2006-01"),
			}
			if d := t.TotalSavedCents - t.TotalTransferredCents; d > 0 {
				out.ToTransferCents = d
			}
			if g, err := s.App.Q.GetGoal(ctx, u.ID); err == nil {
				gb := goalBody(g)
				p := int(t.TotalSavedCents * 100 / g.TargetCents)
				if p > 100 {
					p = 100
				}
				out.Goal = &DashboardGoal{Name: gb.Name, TargetCents: gb.TargetCents, ProgressPercent: p, TargetDate: gb.TargetDate}
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return nil, internalError(s.Log, "dashboard_goal", err)
			}
			if c, err := s.App.Q.GetConnectionByUser(ctx, u.ID); err == nil {
				b := bankState(c, now)
				out.Bank = &b
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return nil, internalError(s.Log, "dashboard_bank", err)
			}
			return &struct{ Body DashboardOut }{out}, nil
		})

	type monthsIn struct {
		Limit int32 `query:"limit" default:"12" minimum:"1" maximum:"24"`
	}
	huma.Register(api, huma.Operation{OperationID: "list-months", Method: http.MethodGet, Path: "/months", Summary: "Épargne et virements par mois", Tags: tag},
		func(ctx context.Context, in *monthsIn) (*struct{ Body []MonthOut }, error) {
			rows, err := s.App.Q.MonthSummaries(ctx, store.MonthSummariesParams{UserID: userFrom(ctx).ID, PageSize: in.Limit})
			if err != nil {
				return nil, internalError(s.Log, "months", err)
			}
			out := make([]MonthOut, 0, len(rows))
			for _, r := range rows {
				out = append(out, MonthOut{Month: r.Month.Format("2006-01"), SavedCents: r.SavedCents,
					TransferredCents: r.TransferredCents, TransactionsCount: r.TransactionsCount})
			}
			return &struct{ Body []MonthOut }{out}, nil
		})

	type txIn struct {
		Month  string `query:"month" doc:"AAAA-MM"`
		Limit  int32  `query:"limit" default:"50" minimum:"1" maximum:"100"`
		Cursor string `query:"cursor"`
	}
	huma.Register(api, huma.Operation{OperationID: "list-transactions", Method: http.MethodGet, Path: "/transactions",
		Summary: "Transactions et leurs arrondis, de la plus récente à la plus ancienne", Tags: tag},
		func(ctx context.Context, in *txIn) (*struct{ Body PageOut[TransactionOut] }, error) {
			p := store.ListTransactionsPageParams{UserID: userFrom(ctx).ID, PageSize: in.Limit + 1}
			if in.Month != "" {
				m, err := time.Parse("2006-01", in.Month)
				if err != nil {
					return nil, huma.Error422UnprocessableEntity("month doit être au format AAAA-MM")
				}
				p.Month = &m
			}
			var err error
			if p.BeforeBookedOn, p.BeforeID, err = decodeCursor(in.Cursor); err != nil {
				return nil, err
			}
			rows, err := s.App.Q.ListTransactionsPage(ctx, p)
			if err != nil {
				return nil, internalError(s.Log, "transactions", err)
			}
			out := PageOut[TransactionOut]{Items: []TransactionOut{}}
			for i, r := range rows {
				if int32(i) == in.Limit {
					last := rows[i-1]
					out.NextCursor = encodeCursor(last.BookedOn, last.ID)
					break
				}
				t := TransactionOut{ID: r.ID, BookedOn: r.BookedOn.Format(dateFmt), AmountCents: r.AmountCents,
					Direction: r.Direction, Currency: r.Currency, Label: r.Label, IsEligible: r.IsEligible,
					ExclusionReason: r.ExclusionReason}
				if r.FinalCents != nil {
					t.Rounding = &RoundingOut{BaseCents: *r.BaseCents, GrossCents: *r.GrossCents, FinalCents: *r.FinalCents, LimitedBy: r.LimitedBy}
				}
				out.Items = append(out.Items, t)
			}
			return &struct{ Body PageOut[TransactionOut] }{out}, nil
		})

	// --- réglages ---

	huma.Register(api, huma.Operation{OperationID: "get-settings", Method: http.MethodGet, Path: "/settings", Summary: "Réglages d'arrondi", Tags: []string{"Réglages"}},
		func(ctx context.Context, _ *struct{}) (*struct{ Body SettingsBody }, error) {
			st, err := s.App.Q.GetSettings(ctx, userFrom(ctx).ID)
			if err != nil {
				return nil, internalError(s.Log, "get_settings", err)
			}
			return &struct{ Body SettingsBody }{settingsBody(st)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "put-settings", Method: http.MethodPut, Path: "/settings",
		Summary: "Modifier les réglages (sans effet rétroactif, D-08)", Tags: []string{"Réglages"}},
		func(ctx context.Context, in *struct{ Body SettingsBody }) (*struct{ Body SettingsBody }, error) {
			b := in.Body
			rs := rounding.Settings{StepCents: b.StepCents, Multiplier: b.Multiplier, TxCapCents: b.TxCapCents}
			if b.MonthCapCents != nil {
				if *b.MonthCapCents == 0 {
					return nil, huma.Error422UnprocessableEntity("month_cap_cents : null pour aucun plafond, sinon 500 à 100000")
				}
				rs.MonthlyCapCents = *b.MonthCapCents
			}
			if err := rs.Validate(); err != nil {
				return nil, huma.Error422UnprocessableEntity(err.Error())
			}
			var monthCap *int32
			if b.MonthCapCents != nil {
				v := int32(*b.MonthCapCents)
				monthCap = &v
			}
			st, err := s.App.Q.UpdateSettings(ctx, store.UpdateSettingsParams{UserID: userFrom(ctx).ID,
				StepCents: int32(b.StepCents), Multiplier: int32(b.Multiplier), TxCapCents: int32(b.TxCapCents), MonthCapCents: monthCap})
			if err != nil {
				return nil, internalError(s.Log, "put_settings", err)
			}
			return &struct{ Body SettingsBody }{settingsBody(st)}, nil
		})

	// --- objectif ---

	goalTag := []string{"Objectif"}
	huma.Register(api, huma.Operation{OperationID: "get-goal", Method: http.MethodGet, Path: "/goal", Summary: "Objectif d'épargne", Tags: goalTag},
		func(ctx context.Context, _ *struct{}) (*struct{ Body GoalBody }, error) {
			g, err := s.App.Q.GetGoal(ctx, userFrom(ctx).ID)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, huma.Error404NotFound("aucun objectif")
			}
			if err != nil {
				return nil, internalError(s.Log, "get_goal", err)
			}
			return &struct{ Body GoalBody }{goalBody(g)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "put-goal", Method: http.MethodPut, Path: "/goal", Summary: "Définir l'objectif", Tags: goalTag},
		func(ctx context.Context, in *struct{ Body GoalBody }) (*struct{ Body GoalBody }, error) {
			if in.Body.TargetCents <= 0 {
				return nil, huma.Error422UnprocessableEntity("target_cents doit être strictement positif")
			}
			p := store.UpsertGoalParams{UserID: userFrom(ctx).ID, Name: strings.TrimSpace(in.Body.Name), TargetCents: in.Body.TargetCents}
			if p.Name == "" {
				return nil, huma.Error422UnprocessableEntity("name ne peut pas être vide")
			}
			if in.Body.TargetDate != nil {
				d, err := time.Parse(dateFmt, *in.Body.TargetDate)
				if err != nil {
					return nil, huma.Error422UnprocessableEntity("target_date doit être au format AAAA-MM-JJ")
				}
				p.TargetDate = &d
			}
			g, err := s.App.Q.UpsertGoal(ctx, p)
			if err != nil {
				return nil, internalError(s.Log, "put_goal", err)
			}
			return &struct{ Body GoalBody }{goalBody(g)}, nil
		})

	huma.Register(api, huma.Operation{OperationID: "delete-goal", Method: http.MethodDelete, Path: "/goal", DefaultStatus: http.StatusNoContent, Summary: "Supprimer l'objectif", Tags: goalTag},
		func(ctx context.Context, _ *struct{}) (*struct{}, error) {
			if err := s.App.Q.DeleteGoal(ctx, userFrom(ctx).ID); err != nil {
				return nil, internalError(s.Log, "delete_goal", err)
			}
			return nil, nil
		})

	// --- virements ---

	trTag := []string{"Virements"}
	type listTrIn struct {
		Limit  int32  `query:"limit" default:"50" minimum:"1" maximum:"100"`
		Cursor string `query:"cursor"`
	}
	huma.Register(api, huma.Operation{OperationID: "list-transfers", Method: http.MethodGet, Path: "/transfers", Summary: "Virements vers l'épargne, du plus récent au plus ancien", Tags: trTag},
		func(ctx context.Context, in *listTrIn) (*struct{ Body PageOut[TransferOut] }, error) {
			p := store.ListTransfersPageParams{UserID: userFrom(ctx).ID, PageSize: in.Limit + 1}
			var err error
			if p.BeforeOn, p.BeforeID, err = decodeCursor(in.Cursor); err != nil {
				return nil, err
			}
			rows, err := s.App.Q.ListTransfersPage(ctx, p)
			if err != nil {
				return nil, internalError(s.Log, "list_transfers", err)
			}
			out := PageOut[TransferOut]{Items: []TransferOut{}}
			for i, r := range rows {
				if int32(i) == in.Limit {
					last := rows[i-1]
					out.NextCursor = encodeCursor(last.TransferredOn, last.ID)
					break
				}
				out.Items = append(out.Items, transferOut(r))
			}
			return &struct{ Body PageOut[TransferOut] }{out}, nil
		})

	type createTrIn struct {
		Body struct {
			AmountCents   int64  `json:"amount_cents"`
			TransferredOn string `json:"transferred_on" format:"date"`
		}
	}
	huma.Register(api, huma.Operation{OperationID: "create-transfer", Method: http.MethodPost, Path: "/transfers", DefaultStatus: http.StatusCreated,
		Summary: "Enregistrer un virement fait à la main", Tags: trTag},
		func(ctx context.Context, in *createTrIn) (*struct{ Body TransferOut }, error) {
			if in.Body.AmountCents <= 0 {
				return nil, huma.Error422UnprocessableEntity("amount_cents doit être strictement positif")
			}
			d, err := time.Parse(dateFmt, in.Body.TransferredOn)
			if err != nil {
				return nil, huma.Error422UnprocessableEntity("transferred_on doit être au format AAAA-MM-JJ")
			}
			if d.After(today(s.now())) {
				return nil, huma.Error422UnprocessableEntity("transferred_on ne peut pas être dans le futur")
			}
			t, err := s.App.Q.CreateTransfer(ctx, store.CreateTransferParams{UserID: userFrom(ctx).ID, AmountCents: in.Body.AmountCents, TransferredOn: d})
			if err != nil {
				return nil, internalError(s.Log, "create_transfer", err)
			}
			return &struct{ Body TransferOut }{transferOut(t)}, nil
		})

	type delTrIn struct {
		ID uuid.UUID `path:"id"`
	}
	huma.Register(api, huma.Operation{OperationID: "delete-transfer", Method: http.MethodDelete, Path: "/transfers/{id}", DefaultStatus: http.StatusNoContent,
		Summary: "Supprimer un virement", Tags: trTag},
		func(ctx context.Context, in *delTrIn) (*struct{}, error) {
			n, err := s.App.Q.DeleteTransfer(ctx, store.DeleteTransferParams{ID: in.ID, UserID: userFrom(ctx).ID})
			if err != nil {
				return nil, internalError(s.Log, "delete_transfer", err)
			}
			if n == 0 {
				return nil, huma.Error404NotFound("virement introuvable")
			}
			return nil, nil
		})
}
