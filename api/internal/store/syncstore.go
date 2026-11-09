package store

// Ce fichier n'est pas généré : il branche le service de synchro (package syncer)
// sur les requêtes sqlc. Le reste du package est produit par `make sqlc`.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/timotheebenchergui/arrondi/api/internal/rounding"
	"github.com/timotheebenchergui/arrondi/api/internal/syncer"
)

// SyncStore implémente syncer.Store sur PostgreSQL.
type SyncStore struct {
	pool *pgxpool.Pool
	q    *Queries
}

func NewSyncStore(pool *pgxpool.Pool) *SyncStore {
	return &SyncStore{pool: pool, q: New(pool)}
}

var _ syncer.Store = (*SyncStore)(nil)

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// SettingsFromRow convertit une ligne user_settings en réglages du moteur
// (month_cap_cents NULL = pas de plafond = 0 côté moteur).
func SettingsFromRow(r UserSetting) rounding.Settings {
	s := rounding.Settings{
		StepCents:  int64(r.StepCents),
		Multiplier: int64(r.Multiplier),
		TxCapCents: int64(r.TxCapCents),
	}
	if r.MonthCapCents != nil {
		s.MonthlyCapCents = int64(*r.MonthCapCents)
	}
	return s
}

func (s *SyncStore) Settings(ctx context.Context, userID uuid.UUID) (rounding.Settings, error) {
	row, err := s.q.GetSettings(ctx, userID)
	if err != nil {
		return rounding.Settings{}, fmt.Errorf("réglages : %w", err)
	}
	return SettingsFromRow(row), nil
}

func (s *SyncStore) LastBookedOn(ctx context.Context, accountID uuid.UUID) (time.Time, bool, error) {
	d, err := s.q.LastBookedOn(ctx, accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	// pgx lit une date à minuit UTC : on garde le jour calendaire.
	return time.Date(d.Year(), d.Month(), d.Day(), 12, 0, 0, 0, time.UTC), true, nil
}

func (s *SyncStore) StartRun(ctx context.Context, connectionID uuid.UUID, trigger string) (uuid.UUID, error) {
	return s.q.StartSyncRun(ctx, StartSyncRunParams{ConnectionID: connectionID, Trigger: trigger})
}

func (s *SyncStore) FinishRun(ctx context.Context, runID uuid.UUID, status string, fetched, inserted int, errMsg string) error {
	return s.q.FinishSyncRun(ctx, FinishSyncRunParams{
		Status: status, FetchedCount: int32(fetched), InsertedCount: int32(inserted),
		Error: strPtr(errMsg), ID: runID,
	})
}

func (s *SyncStore) RecordAttempt(ctx context.Context, connectionID uuid.UUID, ok bool, errMsg string) error {
	return s.q.RecordSyncAttempt(ctx, RecordSyncAttemptParams{Ok: ok, LastError: strPtr(errMsg), ID: connectionID})
}

func (s *SyncStore) SetConnectionStatus(ctx context.Context, connectionID uuid.UUID, status, errMsg string) error {
	return s.q.SetConnectionStatus(ctx, SetConnectionStatusParams{Status: status, LastError: strPtr(errMsg), ID: connectionID})
}

func (s *SyncStore) InUserTx(ctx context.Context, userID uuid.UUID, fn func(syncer.Tx) error) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.q.WithTx(tx)
		if err := q.LockUserForSync(ctx, userID); err != nil {
			return fmt.Errorf("verrou utilisateur : %w", err)
		}
		return fn(syncTx{q})
	})
}

type syncTx struct{ q *Queries }

// dateOnly convertit un jour calendaire (quel que soit son fuseau) en minuit UTC
// du même jour, pour que pgx écrive la bonne date.
func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func (t syncTx) InsertTransaction(ctx context.Context, n syncer.NewTransaction) (uuid.UUID, bool, error) {
	id, err := t.q.InsertTransaction(ctx, InsertTransactionParams{
		AccountID:       n.AccountID,
		ExternalID:      n.ExternalID,
		BookedOn:        dateOnly(n.BookedOn),
		AmountCents:     n.AmountCents,
		Direction:       n.Direction,
		Currency:        n.Currency,
		Label:           n.Label,
		Merchant:        n.Merchant,
		IsEligible:      n.IsEligible,
		ExclusionReason: strPtr(n.ExclusionReason),
		Raw:             n.Raw,
	})
	if errors.Is(err, pgx.ErrNoRows) { // ON CONFLICT DO NOTHING : déjà connue
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	return id, true, nil
}

func (t syncTx) MonthToDateCents(ctx context.Context, userID uuid.UUID, month time.Time) (int64, error) {
	return t.q.MonthToDateCents(ctx, MonthToDateCentsParams{UserID: userID, Month: dateOnly(month)})
}

func (t syncTx) InsertRounding(ctx context.Context, r syncer.NewRounding) error {
	return t.q.InsertRounding(ctx, InsertRoundingParams{
		TransactionID: r.TransactionID,
		UserID:        r.UserID,
		StepCents:     int32(r.Settings.StepCents),
		Multiplier:    int32(r.Settings.Multiplier),
		BaseCents:     int32(r.Result.BaseCents),
		GrossCents:    r.Result.GrossCents,
		FinalCents:    r.Result.FinalCents,
		LimitedBy:     strPtr(string(r.Result.LimitedBy)),
		Month:         dateOnly(r.Month),
	})
}
