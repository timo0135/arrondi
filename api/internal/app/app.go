// Package app regroupe la logique partagée entre l'API HTTP et le planificateur :
// déchiffrement des identifiants, synchronisation d'une connexion et tâche quotidienne.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/timotheebenchergui/arrondi/api/internal/bank"
	"github.com/timotheebenchergui/arrondi/api/internal/enablebanking"
	"github.com/timotheebenchergui/arrondi/api/internal/keybox"
	"github.com/timotheebenchergui/arrondi/api/internal/store"
	"github.com/timotheebenchergui/arrondi/api/internal/syncer"
)

// BankAPI est la partie du client Enable Banking utilisée par l'application.
type BankAPI interface {
	syncer.BankAPI
	ASPSPs(ctx context.Context, cr enablebanking.Credentials, country string) ([]enablebanking.ASPSP, error)
	StartAuth(ctx context.Context, cr enablebanking.Credentials, aspspName, country, redirectURL, state string, validUntil time.Time) (string, error)
	CreateSession(ctx context.Context, cr enablebanking.Credentials, code string) (enablebanking.Session, error)
}

type App struct {
	Pool     *pgxpool.Pool
	Q        *store.Queries
	Box      *keybox.Box
	Bank     BankAPI
	Sync     *syncer.Service
	Profiles []bank.Profile
	Log      *slog.Logger
	Now      func() time.Time
	// RetryDelays : attentes avant chaque nouvelle tentative après une erreur temporaire (section 9.4).
	RetryDelays []time.Duration

	running sync.Map // connection_id → struct{} : une seule synchro à la fois par connexion
}

// New assemble l'application.
func New(pool *pgxpool.Pool, box *keybox.Box, eb BankAPI, log *slog.Logger) *App {
	profiles := []bank.Profile{bank.BoursoBank{}}
	a := &App{
		Pool: pool, Q: store.New(pool), Box: box, Bank: eb, Profiles: profiles, Log: log, Now: time.Now,
		RetryDelays: []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute},
	}
	a.Sync = &syncer.Service{Store: store.NewSyncStore(pool), Bank: eb, Profiles: profiles, Now: func() time.Time { return a.Now() }}
	return a
}

func (a *App) Profile(code string) (bank.Profile, bool) {
	for _, p := range a.Profiles {
		if p.Code() == code {
			return p, true
		}
	}
	return nil, false
}

// KeyAAD lie le chiffré de la clé privée à son utilisateur.
func KeyAAD(userID uuid.UUID) []byte { return userID[:] }

// Credentials déchiffre les identifiants Enable Banking d'une connexion.
func (a *App) Credentials(c store.BankConnection) (enablebanking.Credentials, error) {
	pemBytes, err := a.Box.Open(c.EbPrivateKeyEnc, c.EbKeyNonce, KeyAAD(c.UserID))
	if err != nil {
		return enablebanking.Credentials{}, err
	}
	key, err := enablebanking.ParsePrivateKey(pemBytes)
	if err != nil {
		return enablebanking.Credentials{}, err
	}
	return enablebanking.Credentials{ApplicationID: c.EbApplicationID, PrivateKey: key}, nil
}

var (
	ErrNotActive   = errors.New("la connexion bancaire n'est pas active")
	ErrNoAccount   = errors.New("aucun compte suivi : choisissez-en un")
	ErrSyncRunning = errors.New("une synchronisation est déjà en cours")
)

// PrepareSync vérifie qu'une connexion peut être synchronisée et construit son entrée.
func (a *App) PrepareSync(ctx context.Context, c store.BankConnection) (syncer.Connection, error) {
	if c.Status != "active" || c.ActivatedAt == nil {
		return syncer.Connection{}, ErrNotActive
	}
	acc, err := a.Q.GetTrackedAccount(ctx, c.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return syncer.Connection{}, ErrNoAccount
	}
	if err != nil {
		return syncer.Connection{}, err
	}
	cr, err := a.Credentials(c)
	if err != nil {
		return syncer.Connection{}, err
	}
	return syncer.Connection{
		ID: c.ID, UserID: c.UserID, BankCode: c.BankCode, Credentials: cr,
		ActivatedAt: *c.ActivatedAt, AccountID: acc.ID, AccountUID: acc.EbAccountUid,
	}, nil
}

// SyncConnection synchronise une connexion, avec les nouvelles tentatives de la
// section 9.4 si retry vaut true. Une seule synchro tourne à la fois par connexion.
func (a *App) SyncConnection(ctx context.Context, c store.BankConnection, trigger string, retry bool) (syncer.Result, error) {
	if _, busy := a.running.LoadOrStore(c.ID, struct{}{}); busy {
		return syncer.Result{}, ErrSyncRunning
	}
	defer a.running.Delete(c.ID)

	sc, err := a.PrepareSync(ctx, c)
	if err != nil {
		return syncer.Result{}, err
	}
	for attempt := 0; ; attempt++ {
		res, err := a.Sync.Sync(ctx, sc, trigger)
		log := a.Log.With("connection_id", c.ID, "user_id", c.UserID, "trigger", trigger, "attempt", attempt+1)
		if err == nil {
			log.Info("sync_ok", "fetched", res.Fetched, "inserted", res.Inserted, "rounded", res.Rounded)
			return res, nil
		}
		log.Warn("sync_error", "error", errCode(err))
		if !retry || !syncer.Retryable(err) || attempt >= len(a.RetryDelays) {
			return res, err
		}
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		case <-time.After(a.RetryDelays[attempt]):
		}
	}
}

// errCode renvoie un code d'erreur sans donnée personnelle, pour les journaux.
func errCode(err error) string {
	for _, e := range []error{enablebanking.ErrConsent, enablebanking.ErrCredentials, enablebanking.ErrRateLimited,
		enablebanking.ErrTemporary, enablebanking.ErrBadResponse, ErrNotActive, ErrNoAccount} {
		if errors.Is(err, e) {
			return e.Error()
		}
	}
	return "internal"
}

// RunDaily est la tâche de 06:30 : expiration des consentements, synchronisation
// de toutes les connexions actives (en parallèle), puis purges (D-10, audit).
func (a *App) RunDaily(ctx context.Context) {
	log := a.Log.With("job", "daily")
	if expired, err := a.Q.ExpireConsents(ctx); err != nil {
		log.Error("expire_consents", "error", err.Error())
	} else if len(expired) > 0 {
		log.Info("consents_expired", "count", len(expired))
	}

	conns, err := a.Q.ListConnectionsToSync(ctx)
	if err != nil {
		log.Error("list_connections", "error", err.Error())
		return
	}
	var wg sync.WaitGroup
	for _, c := range conns {
		wg.Add(1)
		go func(c store.BankConnection) {
			defer wg.Done()
			_, _ = a.SyncConnection(ctx, c, syncer.TriggerScheduled, true)
		}(c)
	}
	wg.Wait()

	if n, err := a.Q.PurgeOldTransactions(ctx); err == nil && n > 0 {
		log.Info("purge_transactions", "count", n)
	}
	if n, err := a.Q.PurgeAuditLog(ctx); err == nil && n > 0 {
		log.Info("purge_audit", "count", n)
	}
	log.Info("daily_done", "connections", len(conns))
}

// EnsureUser crée un utilisateur et ses réglages par défaut dans une même transaction.
func (a *App) EnsureUser(ctx context.Context, email, displayName, role string) (store.User, error) {
	var u store.User
	err := pgx.BeginFunc(ctx, a.Pool, func(tx pgx.Tx) error {
		q := a.Q.WithTx(tx)
		var err error
		if u, err = q.CreateUser(ctx, store.CreateUserParams{Email: strings.ToLower(email), DisplayName: displayName, Role: role}); err != nil {
			return err
		}
		_, err = q.CreateDefaultSettings(ctx, u.ID)
		return err
	})
	return u, err
}

// Bootstrap crée l'administrateur si la base ne contient encore aucun utilisateur.
func (a *App) Bootstrap(ctx context.Context, adminEmail string) error {
	if adminEmail == "" {
		return nil
	}
	n, err := a.Q.CountUsers(ctx)
	if err != nil || n > 0 {
		return err
	}
	name, _, _ := strings.Cut(adminEmail, "@")
	if _, err := a.EnsureUser(ctx, adminEmail, name, "admin"); err != nil {
		return fmt.Errorf("création de l'administrateur : %w", err)
	}
	a.Log.Info("bootstrap_admin_created")
	return nil
}
