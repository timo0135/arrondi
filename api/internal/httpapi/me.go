package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/timotheebenchergui/arrondi/api/internal/store"
)

type UserOut struct {
	ID          uuid.UUID `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Role        string    `json:"role" enum:"admin,member"`
}

func userOut(u store.User) UserOut {
	return UserOut{ID: u.ID, Email: u.Email, DisplayName: u.DisplayName, Role: u.Role}
}

type ExportBody struct {
	User         UserOut       `json:"user"`
	Settings     SettingsBody  `json:"settings"`
	Goal         *GoalBody     `json:"goal"`
	Transfers    []TransferOut `json:"transfers"`
	Transactions []ExportTxOut `json:"transactions"`
	ExportedAt   string        `json:"exported_at"`
}

type ExportTxOut struct {
	BookedOn        string  `json:"booked_on"`
	AmountCents     int64   `json:"amount_cents"`
	Direction       string  `json:"direction"`
	Currency        string  `json:"currency"`
	Label           string  `json:"label"`
	IsEligible      bool    `json:"is_eligible"`
	ExclusionReason *string `json:"exclusion_reason"`
	RoundingCents   *int64  `json:"rounding_cents"`
}

func (s *Server) registerMe(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "get-me", Method: http.MethodGet, Path: "/me", Summary: "Utilisateur connecté", Tags: []string{"Compte"}},
		func(ctx context.Context, _ *struct{}) (*struct{ Body UserOut }, error) {
			return &struct{ Body UserOut }{userOut(userFrom(ctx))}, nil
		})

	type deleteIn struct {
		Body struct {
			Confirm bool `json:"confirm" doc:"Doit valoir true"`
		}
	}
	huma.Register(api, huma.Operation{OperationID: "delete-me", Method: http.MethodDelete, Path: "/me", DefaultStatus: http.StatusNoContent,
		Summary: "Supprimer son compte et toutes ses données (EF-30)", Tags: []string{"Compte"}},
		func(ctx context.Context, in *deleteIn) (*struct{}, error) {
			if !in.Body.Confirm {
				return nil, huma.Error422UnprocessableEntity("confirm doit valoir true")
			}
			u := userFrom(ctx)
			if err := s.App.Q.DeleteUser(ctx, u.ID); err != nil {
				return nil, internalError(s.Log, "delete_user", err)
			}
			s.Log.Info("user_deleted", "user_id", u.ID)
			return nil, nil
		})

	type exportOut struct {
		ContentDisposition string `header:"Content-Disposition"`
		Body               ExportBody
	}
	huma.Register(api, huma.Operation{OperationID: "export-me", Method: http.MethodGet, Path: "/me/export",
		Summary: "Exporter toutes ses données (EF-29)", Tags: []string{"Compte"}},
		func(ctx context.Context, _ *struct{}) (*exportOut, error) {
			u := userFrom(ctx)
			q := s.App.Q
			st, err := q.GetSettings(ctx, u.ID)
			if err != nil {
				return nil, internalError(s.Log, "export_settings", err)
			}
			out := ExportBody{User: userOut(u), Settings: settingsBody(st), Transfers: []TransferOut{}, Transactions: []ExportTxOut{},
				ExportedAt: s.now().UTC().Format("2006-01-02T15:04:05Z")}
			if g, err := q.GetGoal(ctx, u.ID); err == nil {
				gb := goalBody(g)
				out.Goal = &gb
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return nil, internalError(s.Log, "export_goal", err)
			}
			trs, err := q.ListTransfers(ctx, u.ID)
			if err != nil {
				return nil, internalError(s.Log, "export_transfers", err)
			}
			for _, t := range trs {
				out.Transfers = append(out.Transfers, transferOut(t))
			}
			txs, err := q.ExportTransactions(ctx, u.ID)
			if err != nil {
				return nil, internalError(s.Log, "export_transactions", err)
			}
			for _, t := range txs {
				out.Transactions = append(out.Transactions, ExportTxOut{
					BookedOn: t.BookedOn.Format(dateFmt), AmountCents: t.AmountCents, Direction: t.Direction,
					Currency: t.Currency, Label: t.Label, IsEligible: t.IsEligible, ExclusionReason: t.ExclusionReason,
					RoundingCents: t.FinalCents,
				})
			}
			return &exportOut{ContentDisposition: `attachment; filename="arrondi-export.json"`, Body: out}, nil
		})
}

// --- administration ---

type AdminUserOut struct {
	UserOut
	IsActive         bool    `json:"is_active"`
	ConnectionStatus *string `json:"connection_status" doc:"pending, active, expired, error, ou null sans connexion"`
}

func requireAdmin(ctx context.Context) error {
	if userFrom(ctx).Role != "admin" {
		return huma.Error403Forbidden("réservé à l'administrateur")
	}
	return nil
}

func (s *Server) registerAdmin(api huma.API) {
	huma.Register(api, huma.Operation{OperationID: "admin-list-users", Method: http.MethodGet, Path: "/admin/users", Summary: "Lister les utilisateurs", Tags: []string{"Administration"}},
		func(ctx context.Context, _ *struct{}) (*struct{ Body []AdminUserOut }, error) {
			if err := requireAdmin(ctx); err != nil {
				return nil, err
			}
			rows, err := s.App.Q.ListUsersWithConnection(ctx)
			if err != nil {
				return nil, internalError(s.Log, "admin_list", err)
			}
			out := make([]AdminUserOut, 0, len(rows))
			for _, r := range rows {
				out = append(out, AdminUserOut{
					UserOut:  UserOut{ID: r.ID, Email: r.Email, DisplayName: r.DisplayName, Role: r.Role},
					IsActive: r.IsActive, ConnectionStatus: r.ConnectionStatus,
				})
			}
			return &struct{ Body []AdminUserOut }{out}, nil
		})

	type createIn struct {
		Body struct {
			Email       string `json:"email" format:"email" maxLength:"254"`
			DisplayName string `json:"display_name" minLength:"1" maxLength:"60"`
		}
	}
	huma.Register(api, huma.Operation{OperationID: "admin-create-user", Method: http.MethodPost, Path: "/admin/users", DefaultStatus: http.StatusCreated,
		Summary: "Inviter un utilisateur (rôle member)", Tags: []string{"Administration"}},
		func(ctx context.Context, in *createIn) (*struct{ Body AdminUserOut }, error) {
			if err := requireAdmin(ctx); err != nil {
				return nil, err
			}
			u, err := s.App.EnsureUser(ctx, in.Body.Email, in.Body.DisplayName, "member")
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return nil, huma.Error409Conflict("cet email existe déjà")
			}
			if err != nil {
				return nil, internalError(s.Log, "admin_create", err)
			}
			s.Log.Info("user_created", "user_id", u.ID)
			return &struct{ Body AdminUserOut }{AdminUserOut{UserOut: userOut(u), IsActive: u.IsActive}}, nil
		})

	type patchIn struct {
		ID   uuid.UUID `path:"id"`
		Body struct {
			IsActive bool `json:"is_active"`
		}
	}
	huma.Register(api, huma.Operation{OperationID: "admin-patch-user", Method: http.MethodPatch, Path: "/admin/users/{id}",
		Summary: "Activer ou désactiver un utilisateur", Tags: []string{"Administration"}},
		func(ctx context.Context, in *patchIn) (*struct{ Body AdminUserOut }, error) {
			if err := requireAdmin(ctx); err != nil {
				return nil, err
			}
			if in.ID == userFrom(ctx).ID && !in.Body.IsActive {
				return nil, huma.Error409Conflict("l'administrateur ne peut pas se désactiver lui-même")
			}
			if err := s.App.Q.SetUserActive(ctx, store.SetUserActiveParams{ID: in.ID, IsActive: in.Body.IsActive}); err != nil {
				return nil, internalError(s.Log, "admin_patch", err)
			}
			u, err := s.App.Q.GetUser(ctx, in.ID)
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, huma.Error404NotFound("utilisateur introuvable")
			}
			if err != nil {
				return nil, internalError(s.Log, "admin_patch_get", err)
			}
			return &struct{ Body AdminUserOut }{AdminUserOut{UserOut: userOut(u), IsActive: u.IsActive}}, nil
		})
}
