package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/timotheebenchergui/arrondi/api/internal/app"
	"github.com/timotheebenchergui/arrondi/api/internal/syncer"
)

// registerDev ajoute les routes réservées au développement local (APP_ENV=dev).
// Elles ne sont jamais enregistrées en production.
func (s *Server) registerDev(api huma.API) {
	type backfillIn struct {
		Body struct {
			Days int `json:"days" minimum:"1" maximum:"90" default:"30" doc:"Nombre de jours à reprendre (90 au plus : au-delà, la banque exige une nouvelle authentification)"`
		}
	}
	type backfillOut struct {
		Body struct {
			ActivatedAt time.Time `json:"activated_at"`
			Fetched     int       `json:"fetched"`
			Inserted    int       `json:"inserted"`
			Rounded     int       `json:"rounded"`
			SavedCents  int64     `json:"saved_cents"`
		}
	}
	huma.Register(api, huma.Operation{OperationID: "dev-backfill", Method: http.MethodPost, Path: "/dev/backfill",
		Summary: "DEV : reculer la date d'activation et réimporter l'historique",
		Description: "Efface les opérations importées de l'utilisateur, recule activated_at de `days` jours " +
			"et lance une synchronisation immédiate, sans quota. Route absente en production.",
		Tags: []string{"Dev"}},
		func(ctx context.Context, in *backfillIn) (*backfillOut, error) {
			u := userFrom(ctx)
			c, err := s.connection(ctx)
			if err != nil {
				return nil, err
			}
			if c.Status != "active" || c.ActivatedAt == nil {
				return nil, huma.Error409Conflict("la connexion bancaire n'est pas active")
			}
			days := in.Body.Days
			if days == 0 {
				days = 30
			}
			// Jour d'activation = aujourd'hui (Paris) moins days, à midi pour rester sur le bon jour.
			now := s.now().In(paris)
			activated := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, paris).AddDate(0, 0, -days)

			tx, err := s.App.Pool.Begin(ctx)
			if err != nil {
				return nil, internalError(s.Log, "dev_backfill_begin", err)
			}
			defer tx.Rollback(context.WithoutCancel(ctx))
			if _, err := tx.Exec(ctx, `DELETE FROM transactions t USING bank_accounts a
				WHERE t.account_id = a.id AND a.connection_id = $1`, c.ID); err != nil {
				return nil, internalError(s.Log, "dev_backfill_delete", err)
			}
			if _, err := tx.Exec(ctx, `DELETE FROM sync_runs WHERE connection_id = $1`, c.ID); err != nil {
				return nil, internalError(s.Log, "dev_backfill_runs", err)
			}
			if _, err := tx.Exec(ctx, `UPDATE bank_connections SET activated_at = $2 WHERE id = $1`, c.ID, activated); err != nil {
				return nil, internalError(s.Log, "dev_backfill_update", err)
			}
			if err := tx.Commit(ctx); err != nil {
				return nil, internalError(s.Log, "dev_backfill_commit", err)
			}
			s.Log.Warn("dev_backfill", "user_id", u.ID, "days", days)

			c, err = s.connection(ctx)
			if err != nil {
				return nil, err
			}
			res, err := s.App.SyncConnection(ctx, c, syncer.TriggerManual, false)
			if errors.Is(err, app.ErrSyncRunning) {
				return nil, huma.Error409Conflict("une synchronisation est déjà en cours, réessayez dans un instant")
			}
			if err != nil {
				s.Log.Warn("dev_backfill_sync_failed", "user_id", u.ID, "error", err.Error())
				return nil, bankError(err)
			}
			out := &backfillOut{}
			out.Body.ActivatedAt = activated.UTC()
			out.Body.Fetched, out.Body.Inserted, out.Body.Rounded, out.Body.SavedCents = res.Fetched, res.Inserted, res.Rounded, res.SavedCents
			return out, nil
		})
}
