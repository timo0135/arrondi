// Package syncer importe les transactions d'une connexion bancaire et calcule
// leurs arrondis (cahier des charges, section 9.3, diagramme S2).
//
// Il ne connaît ni SQL ni HTTP : la base et l'API bancaire sont derrière les
// interfaces Store et BankAPI, ce qui le rend testable sans rien démarrer.
package syncer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/timotheebenchergui/arrondi/api/internal/bank"
	"github.com/timotheebenchergui/arrondi/api/internal/enablebanking"
	"github.com/timotheebenchergui/arrondi/api/internal/rounding"
)

// Trigger d'une exécution.
const (
	TriggerScheduled = "scheduled"
	TriggerManual    = "manual"
)

// Connection regroupe ce que la synchro doit savoir d'une connexion active.
// La clé privée est déjà déchiffrée par l'appelant.
type Connection struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	BankCode    string
	Credentials enablebanking.Credentials
	ActivatedAt time.Time
	AccountID   uuid.UUID // compte suivi
	AccountUID  string    // uid Enable Banking du compte suivi
}

// BankAPI lit les transactions comptabilisées d'un compte.
type BankAPI interface {
	Transactions(ctx context.Context, cr enablebanking.Credentials, accountUID string, from, to time.Time) ([]enablebanking.Item, error)
}

// NewTransaction est une ligne de la table transactions.
type NewTransaction struct {
	AccountID       uuid.UUID
	ExternalID      string
	BookedOn        time.Time
	AmountCents     int64
	Direction       string // "debit" | "credit"
	Currency        string
	Label           string
	IsEligible      bool
	ExclusionReason string // "" si éligible
	Raw             json.RawMessage
}

// NewRounding est une ligne de la table roundings.
type NewRounding struct {
	TransactionID uuid.UUID
	UserID        uuid.UUID
	Settings      rounding.Settings
	Result        rounding.Result
	Month         time.Time
}

// Tx est l'accès à la base dans une transaction SQL verrouillée sur l'utilisateur.
type Tx interface {
	// InsertTransaction renvoie inserted=false si la transaction existait déjà.
	InsertTransaction(ctx context.Context, t NewTransaction) (id uuid.UUID, inserted bool, err error)
	MonthToDateCents(ctx context.Context, userID uuid.UUID, month time.Time) (int64, error)
	InsertRounding(ctx context.Context, r NewRounding) error
}

// Store est l'accès à la base hors transaction.
type Store interface {
	Settings(ctx context.Context, userID uuid.UUID) (rounding.Settings, error)
	// LastBookedOn renvoie ok=false si le compte n'a encore aucune transaction.
	LastBookedOn(ctx context.Context, accountID uuid.UUID) (day time.Time, ok bool, err error)
	StartRun(ctx context.Context, connectionID uuid.UUID, trigger string) (uuid.UUID, error)
	FinishRun(ctx context.Context, runID uuid.UUID, status string, fetched, inserted int, errMsg string) error
	RecordAttempt(ctx context.Context, connectionID uuid.UUID, ok bool, errMsg string) error
	SetConnectionStatus(ctx context.Context, connectionID uuid.UUID, status, errMsg string) error
	// InUserTx ouvre une transaction SQL, prend le verrou de l'utilisateur,
	// exécute fn puis valide ; toute erreur annule tout le lot.
	InUserTx(ctx context.Context, userID uuid.UUID, fn func(Tx) error) error
}

// Result résume une exécution.
type Result struct {
	Fetched    int // transactions lues chez la banque
	Inserted   int // nouvelles transactions enregistrées
	Rounded    int // nouveaux arrondis
	SavedCents int64
}

// Service orchestre la synchronisation.
type Service struct {
	Store    Store
	Bank     BankAPI
	Profiles []bank.Profile
	Now      func() time.Time // horloge, remplaçable dans les tests
}

var paris = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		panic(err)
	}
	return loc
}()

// Day ramène un instant au jour calendaire à minuit, Europe/Paris.
func Day(t time.Time) time.Time {
	y, m, d := t.In(paris).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, paris)
}

// Month renvoie le premier jour du mois d'un jour calendaire.
func Month(day time.Time) time.Time {
	y, m, _ := day.Date()
	return time.Date(y, m, 1, 0, 0, 0, 0, day.Location())
}

// relecture : nombre de jours relus avant la dernière transaction connue (section 9.3).
const overlapDays = 7

// Sync importe les transactions de la connexion. Une erreur bancaire met à jour
// le statut de la connexion selon la section 9.4 ; dans tous les cas l'exécution
// est tracée dans sync_runs. Aucune donnée n'est écrite si une seule transaction
// est illisible (EF-13).
func (s *Service) Sync(ctx context.Context, c Connection, trigger string) (Result, error) {
	runID, err := s.Store.StartRun(ctx, c.ID, trigger)
	if err != nil {
		return Result{}, fmt.Errorf("début d'exécution : %w", err)
	}
	res, err := s.run(ctx, c)
	if err != nil {
		msg := publicMessage(err)
		s.onError(ctx, c, err, msg)
		_ = s.Store.FinishRun(ctx, runID, "error", res.Fetched, 0, msg)
		return res, err
	}
	if err := s.Store.RecordAttempt(ctx, c.ID, true, ""); err != nil {
		return res, err
	}
	return res, s.Store.FinishRun(ctx, runID, "success", res.Fetched, res.Inserted, "")
}

func (s *Service) run(ctx context.Context, c Connection) (Result, error) {
	profile, ok := s.profile(c.BankCode)
	if !ok {
		return Result{}, fmt.Errorf("%w : profil de banque %q inconnu", errInternal, c.BankCode)
	}
	settings, err := s.Store.Settings(ctx, c.UserID)
	if err != nil {
		return Result{}, err
	}
	if err := settings.Validate(); err != nil {
		return Result{}, err
	}

	now := s.Now()
	activatedOn := Day(c.ActivatedAt)
	from := activatedOn
	if last, ok, err := s.Store.LastBookedOn(ctx, c.AccountID); err != nil {
		return Result{}, err
	} else if ok {
		if f := Day(last).AddDate(0, 0, -overlapDays); f.After(from) {
			from = f
		}
	}
	to := Day(now)

	items, err := s.Bank.Transactions(ctx, c.Credentials, c.AccountUID, from, to)
	res := Result{Fetched: len(items)}
	if err != nil {
		return res, err
	}

	// Tout est normalisé avant la moindre écriture.
	type entry struct {
		tx  bank.Transaction
		raw json.RawMessage
	}
	entries := make([]entry, 0, len(items))
	for _, it := range items {
		t, err := bank.Normalize(profile, it.Tx)
		if errors.Is(err, bank.ErrSkip) {
			continue
		}
		if err != nil {
			return res, fmt.Errorf("%w : %v", enablebanking.ErrBadResponse, err)
		}
		entries = append(entries, entry{t, it.Raw})
	}
	// Ordre d'enregistrement déterministe : du plus ancien au plus récent.
	sort.SliceStable(entries, func(i, j int) bool {
		if !entries[i].tx.BookedOn.Equal(entries[j].tx.BookedOn) {
			return entries[i].tx.BookedOn.Before(entries[j].tx.BookedOn)
		}
		return entries[i].tx.ExternalID < entries[j].tx.ExternalID
	})

	err = s.Store.InUserTx(ctx, c.UserID, func(db Tx) error {
		for _, e := range entries {
			reason := Eligibility(e.tx, activatedOn, true)
			direction := "credit"
			if e.tx.Debit {
				direction = "debit"
			}
			id, inserted, err := db.InsertTransaction(ctx, NewTransaction{
				AccountID:       c.AccountID,
				ExternalID:      e.tx.ExternalID,
				BookedOn:        e.tx.BookedOn,
				AmountCents:     e.tx.AmountCents,
				Direction:       direction,
				Currency:        e.tx.Currency,
				Label:           e.tx.Label,
				IsEligible:      reason == "",
				ExclusionReason: reason,
				Raw:             e.raw,
			})
			if err != nil {
				return err
			}
			if !inserted {
				continue // déjà connue : jamais arrondie deux fois
			}
			res.Inserted++
			if reason != "" {
				continue
			}
			month := Month(e.tx.BookedOn)
			mtd, err := db.MonthToDateCents(ctx, c.UserID, month)
			if err != nil {
				return err
			}
			r := rounding.Compute(settings, e.tx.AmountCents, mtd)
			if err := db.InsertRounding(ctx, NewRounding{
				TransactionID: id, UserID: c.UserID, Settings: settings, Result: r, Month: month,
			}); err != nil {
				return err
			}
			res.Rounded++
			res.SavedCents += r.FinalCents
		}
		return nil
	})
	if err != nil {
		res.Inserted, res.Rounded, res.SavedCents = 0, 0, 0 // lot annulé
	}
	return res, err
}

func (s *Service) profile(code string) (bank.Profile, bool) {
	for _, p := range s.Profiles {
		if p.Code() == code {
			return p, true
		}
	}
	return nil, false
}

var errInternal = errors.New("erreur interne")

// onError applique la section 9.4 au statut de la connexion.
func (s *Service) onError(ctx context.Context, c Connection, err error, msg string) {
	_ = s.Store.RecordAttempt(ctx, c.ID, false, msg)
	switch {
	case errors.Is(err, enablebanking.ErrConsent):
		_ = s.Store.SetConnectionStatus(ctx, c.ID, "expired", msg)
	case errors.Is(err, enablebanking.ErrCredentials), errors.Is(err, enablebanking.ErrBadResponse):
		_ = s.Store.SetConnectionStatus(ctx, c.ID, "error", msg)
		// ErrRateLimited et ErrTemporary : la connexion reste active,
		// le planificateur décide d'une nouvelle tentative.
	}
}

// Retryable dit si une erreur justifie une nouvelle tentative (1, 5 puis 15 minutes).
func Retryable(err error) bool { return errors.Is(err, enablebanking.ErrTemporary) }

// publicMessage renvoie un message sans donnée personnelle (stocké en base).
func publicMessage(err error) string {
	for _, e := range []error{
		enablebanking.ErrConsent, enablebanking.ErrCredentials, enablebanking.ErrRateLimited,
		enablebanking.ErrTemporary, enablebanking.ErrBadResponse,
	} {
		if errors.Is(err, e) {
			return e.Error()
		}
	}
	return "erreur interne de synchronisation"
}
