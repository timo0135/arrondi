package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/timotheebenchergui/arrondi/api/internal/app"
	"github.com/timotheebenchergui/arrondi/api/internal/bank"
	"github.com/timotheebenchergui/arrondi/api/internal/enablebanking"
	"github.com/timotheebenchergui/arrondi/api/internal/store"
	"github.com/timotheebenchergui/arrondi/api/internal/syncer"
)

type AccountOut struct {
	ID        uuid.UUID `json:"id"`
	Label     *string   `json:"label"`
	IBANLast4 *string   `json:"iban_last4"`
	Currency  string    `json:"currency"`
	IsTracked bool      `json:"is_tracked"`
}

type ConnectionOut struct {
	BankCode          string       `json:"bank_code"`
	ASPSPName         string       `json:"aspsp_name"`
	Status            string       `json:"status" enum:"pending,active,expiring_soon,expired,error"`
	ConsentValidUntil *time.Time   `json:"consent_valid_until"`
	ExpiresInDays     *int         `json:"expires_in_days"`
	ActivatedAt       *time.Time   `json:"activated_at"`
	LastSyncAt        *time.Time   `json:"last_sync_at"`
	LastSyncOkAt      *time.Time   `json:"last_sync_ok_at"`
	LastError         *string      `json:"last_error"`
	Accounts          []AccountOut `json:"accounts"`
}

const authStateTTL = 10 * time.Minute

func (s *Server) connection(ctx context.Context) (store.BankConnection, error) {
	c, err := s.App.Q.GetConnectionByUser(ctx, userFrom(ctx).ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, huma.Error404NotFound("aucune connexion bancaire : enregistrez d'abord vos identifiants Enable Banking")
	}
	if err != nil {
		return c, internalError(s.Log, "get_connection", err)
	}
	return c, nil
}

// bankError traduit une erreur Enable Banking en réponse HTTP (codes de la section 7).
func bankError(err error) error {
	switch {
	case errors.Is(err, enablebanking.ErrCredentials):
		return huma.Error422UnprocessableEntity("identifiant ou clé refusés par Enable Banking")
	case errors.Is(err, enablebanking.ErrRateLimited):
		return huma.Error429TooManyRequests("la banque limite le nombre de lectures, réessayez plus tard")
	default:
		return huma.Error502BadGateway("la banque ou Enable Banking ne répond pas")
	}
}

func (s *Server) registerBank(api huma.API) {
	tag := []string{"Banque"}

	huma.Register(api, huma.Operation{OperationID: "get-bank-connection", Method: http.MethodGet, Path: "/bank/connection", Summary: "État de la connexion bancaire", Tags: tag},
		func(ctx context.Context, _ *struct{}) (*struct{ Body ConnectionOut }, error) {
			c, err := s.connection(ctx)
			if err != nil {
				return nil, err
			}
			st := bankState(c, s.now())
			out := ConnectionOut{BankCode: c.BankCode, ASPSPName: c.AspspName, Status: st.Status, ConsentValidUntil: utc(c.ConsentValidUntil),
				ExpiresInDays: st.ExpiresInDays, ActivatedAt: utc(c.ActivatedAt), LastSyncAt: utc(c.LastSyncAt), LastSyncOkAt: utc(c.LastSyncOkAt),
				LastError: c.LastError, Accounts: []AccountOut{}}
			accs, err := s.App.Q.ListAccounts(ctx, c.ID)
			if err != nil {
				return nil, internalError(s.Log, "list_accounts", err)
			}
			for _, a := range accs {
				out.Accounts = append(out.Accounts, AccountOut{ID: a.ID, Label: a.Label, IBANLast4: a.IbanLast4, Currency: a.Currency, IsTracked: a.IsTracked})
			}
			return &struct{ Body ConnectionOut }{out}, nil
		})

	type credIn struct {
		Body struct {
			BankCode        string `json:"bank_code" enum:"boursobank" doc:"Profil de banque (D-17)"`
			ASPSPName       string `json:"aspsp_name,omitempty" required:"false" doc:"Nom exact dans GET /aspsps, pour les banques à plusieurs caisses"`
			EbApplicationID string `json:"eb_application_id" minLength:"1" maxLength:"200"`
			PrivateKeyPEM   string `json:"private_key_pem" minLength:"1" maxLength:"16384"`
		}
	}
	huma.Register(api, huma.Operation{OperationID: "put-bank-credentials", Method: http.MethodPut, Path: "/bank/credentials", DefaultStatus: http.StatusNoContent,
		Summary: "Enregistrer ses identifiants Enable Banking (la clé n'est jamais renvoyée)", Tags: tag},
		func(ctx context.Context, in *credIn) (*struct{}, error) {
			u := userFrom(ctx)
			b := in.Body
			profile, ok := s.App.Profile(b.BankCode)
			if !ok {
				return nil, huma.Error422UnprocessableEntity("banque non prise en charge")
			}
			aspsp := strings.TrimSpace(b.ASPSPName)
			if aspsp == "" {
				if dn, ok := profile.(bank.DefaultNamer); ok {
					aspsp = dn.DefaultASPSPName()
				} else {
					return nil, huma.Error422UnprocessableEntity("aspsp_name est obligatoire pour cette banque")
				}
			}
			if !profile.Recognizes(aspsp) {
				return nil, huma.Error422UnprocessableEntity("aspsp_name ne correspond pas à cette banque")
			}
			if _, err := enablebanking.ParsePrivateKey([]byte(b.PrivateKeyPEM)); err != nil {
				return nil, huma.Error422UnprocessableEntity("private_key_pem n'est pas une clé privée RSA valide")
			}
			if existing, err := s.App.Q.GetConnectionByUser(ctx, u.ID); err == nil && existing.BankCode != b.BankCode {
				return nil, huma.Error409Conflict("supprimez d'abord la connexion actuelle pour changer de banque")
			}
			enc, nonce, err := s.App.Box.Seal([]byte(b.PrivateKeyPEM), app.KeyAAD(u.ID))
			if err != nil {
				return nil, internalError(s.Log, "seal_key", err)
			}
			if _, err := s.App.Q.UpsertCredentials(ctx, store.UpsertCredentialsParams{UserID: u.ID, BankCode: b.BankCode,
				EbApplicationID: strings.TrimSpace(b.EbApplicationID), EbPrivateKeyEnc: enc, EbKeyNonce: nonce, AspspName: aspsp}); err != nil {
				return nil, internalError(s.Log, "upsert_credentials", err)
			}
			s.Log.Info("bank_credentials_saved", "user_id", u.ID)
			return nil, nil
		})

	type authOut struct {
		Body struct {
			URL string `json:"url" doc:"Adresse de la banque où rediriger l'utilisateur"`
		}
	}
	huma.Register(api, huma.Operation{OperationID: "create-bank-authorization", Method: http.MethodPost, Path: "/bank/authorizations",
		Summary: "Démarrer l'autorisation chez la banque (section 9.2)", Tags: tag},
		func(ctx context.Context, _ *struct{}) (*authOut, error) {
			u := userFrom(ctx)
			c, err := s.connection(ctx)
			if err != nil {
				return nil, err
			}
			profile, ok := s.App.Profile(c.BankCode)
			if !ok {
				return nil, internalError(s.Log, "unknown_profile", errors.New(c.BankCode))
			}
			cr, err := s.App.Credentials(c)
			if err != nil {
				return nil, internalError(s.Log, "credentials", err)
			}
			raw := make([]byte, 32)
			if _, err := rand.Read(raw); err != nil {
				return nil, internalError(s.Log, "state", err)
			}
			state := base64.RawURLEncoding.EncodeToString(raw)
			exp := s.now().Add(authStateTTL)
			n, err := s.App.Q.BeginAuthorization(ctx, store.BeginAuthorizationParams{UserID: u.ID, AuthState: &state, AuthStateExpiresAt: &exp})
			if err != nil {
				return nil, internalError(s.Log, "begin_auth", err)
			}
			if n == 0 {
				return nil, huma.Error409Conflict("une autorisation est déjà en cours, réessayez dans 10 minutes")
			}
			fail := func(e error) (*authOut, error) {
				_ = s.App.Q.CancelAuthorization(context.WithoutCancel(ctx), u.ID)
				return nil, e
			}

			// Durée de consentement : le maximum de la banque, borné par le profil.
			validity := profile.MaxConsent()
			aspsps, err := s.App.Bank.ASPSPs(ctx, cr, "FR")
			if err != nil {
				return fail(bankError(err))
			}
			found := false
			for _, a := range aspsps {
				if strings.EqualFold(a.Name, c.AspspName) {
					found = true
					if d := time.Duration(a.MaximumConsentValidity) * time.Second; d > 0 && d < validity {
						validity = d
					}
				}
			}
			if !found {
				return fail(huma.Error502BadGateway("la banque n'est plus proposée par Enable Banking"))
			}
			url, err := s.App.Bank.StartAuth(ctx, cr, c.AspspName, "FR", s.RedirectURL(), state, s.now().Add(validity))
			if err != nil {
				return fail(bankError(err))
			}
			s.Log.Info("bank_auth_started", "user_id", u.ID)
			out := &authOut{}
			out.Body.URL = url
			return out, nil
		})

	type accountIn struct {
		Body struct {
			AccountID uuid.UUID `json:"account_id"`
		}
	}
	huma.Register(api, huma.Operation{OperationID: "put-bank-account", Method: http.MethodPut, Path: "/bank/account", DefaultStatus: http.StatusNoContent,
		Summary: "Choisir le compte suivi", Tags: tag},
		func(ctx context.Context, in *accountIn) (*struct{}, error) {
			u := userFrom(ctx)
			acc, err := s.App.Q.GetAccountForUser(ctx, store.GetAccountForUserParams{AccountID: in.Body.AccountID, UserID: u.ID})
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, huma.Error404NotFound("compte introuvable")
			}
			if err != nil {
				return nil, internalError(s.Log, "get_account", err)
			}
			if err := pgx.BeginFunc(ctx, s.App.Pool, func(tx pgx.Tx) error {
				q := s.App.Q.WithTx(tx)
				if err := q.UntrackAccounts(ctx, acc.ConnectionID); err != nil {
					return err
				}
				return q.TrackAccount(ctx, store.TrackAccountParams{ID: acc.ID, ConnectionID: acc.ConnectionID})
			}); err != nil {
				return nil, internalError(s.Log, "track_account", err)
			}
			return nil, nil
		})

	type syncOut struct {
		Body struct {
			Status string `json:"status" enum:"accepted"`
		}
	}
	huma.Register(api, huma.Operation{OperationID: "sync-bank", Method: http.MethodPost, Path: "/bank/sync", DefaultStatus: http.StatusAccepted,
		Summary: "Lancer une synchronisation (1 par heure, 4 sur 24 heures, EF-10)", Tags: tag},
		func(ctx context.Context, _ *struct{}) (*syncOut, error) {
			c, err := s.connection(ctx)
			if err != nil {
				return nil, err
			}
			if c.Status != "active" {
				return nil, huma.Error409Conflict("la connexion bancaire n'est pas active")
			}
			if _, err := s.App.Q.GetTrackedAccount(ctx, c.ID); errors.Is(err, pgx.ErrNoRows) {
				return nil, huma.Error409Conflict("choisissez d'abord le compte suivi")
			}
			now := s.now()
			for _, lim := range []struct {
				since time.Duration
				max   int32
			}{{time.Hour, 1}, {24 * time.Hour, 4}} {
				n, err := s.App.Q.CountSyncRunsSince(ctx, store.CountSyncRunsSinceParams{ConnectionID: c.ID, Since: now.Add(-lim.since)})
				if err != nil {
					return nil, internalError(s.Log, "count_syncs", err)
				}
				if n >= lim.max {
					return nil, huma.Error429TooManyRequests("limite de synchronisations atteinte (1 par heure, 4 sur 24 heures)")
				}
			}
			go func() {
				bg, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()
				_, _ = s.App.SyncConnection(bg, c, syncer.TriggerManual, false)
			}()
			out := &syncOut{}
			out.Body.Status = "accepted"
			return out, nil
		})

	huma.Register(api, huma.Operation{OperationID: "delete-bank-connection", Method: http.MethodDelete, Path: "/bank/connection", DefaultStatus: http.StatusNoContent,
		Summary: "Supprimer la connexion et toutes les transactions (EF-08)", Tags: tag},
		func(ctx context.Context, _ *struct{}) (*struct{}, error) {
			u := userFrom(ctx)
			if _, err := s.App.Q.DeleteConnection(ctx, u.ID); err != nil {
				return nil, internalError(s.Log, "delete_connection", err)
			}
			s.Log.Info("bank_connection_deleted", "user_id", u.ID)
			return nil, nil
		})
}

// bankCallback : la banque renvoie le navigateur ici (section 9.2, étapes 4 et 5).
// Toujours une redirection vers l'écran Banque de la PWA, sauf état invalide (400).
func (s *Server) bankCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	q := r.URL.Query()
	state := q.Get("state")
	if state == "" {
		writeProblem(w, http.StatusBadRequest, "state manquant")
		return
	}
	c, err := s.App.Q.ConsumeAuthState(ctx, store.ConsumeAuthStateParams{UserID: u.ID, AuthState: &state})
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusBadRequest, "state inconnu, expiré ou déjà utilisé")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "erreur interne")
		return
	}
	back := func(status string) {
		http.Redirect(w, r, s.PublicBaseURL+"/reglages/banque?status="+status, http.StatusFound)
	}
	code := q.Get("code")
	if code == "" { // refus ou annulation chez la banque (?error=...)
		s.Log.Info("bank_auth_refused", "user_id", u.ID)
		back("refused")
		return
	}
	if err := s.activate(ctx, c, code); err != nil {
		s.Log.Warn("bank_auth_failed", "user_id", u.ID, "error", err.Error())
		back("error")
		return
	}
	s.Log.Info("bank_auth_ok", "user_id", u.ID)
	back("ok")
}

// activate crée la session, enregistre les comptes et, s'il n'y a qu'un compte
// synchronisable et aucun compte suivi, le suit automatiquement.
func (s *Server) activate(ctx context.Context, c store.BankConnection, code string) error {
	cr, err := s.App.Credentials(c)
	if err != nil {
		return err
	}
	sess, err := s.App.Bank.CreateSession(ctx, cr, code)
	if err != nil {
		return err
	}
	profile, ok := s.App.Profile(c.BankCode)
	if !ok {
		return errors.New("profil inconnu")
	}
	return pgx.BeginFunc(ctx, s.App.Pool, func(tx pgx.Tx) error {
		q := s.App.Q.WithTx(tx)
		valid := sess.ValidUntil
		if _, err := q.ActivateConnection(ctx, store.ActivateConnectionParams{ID: c.ID, EbSessionID: &sess.ID, ConsentValidUntil: &valid}); err != nil {
			return err
		}
		var syncable []store.BankAccount
		for _, a := range sess.Accounts {
			p := store.UpsertAccountParams{ConnectionID: c.ID, EbAccountUid: a.UID, Currency: accountCurrency(sess.Currencies[a.UID])}
			if iban := strings.ReplaceAll(sess.IBANs[a.UID], " ", ""); len(iban) >= 4 {
				last := iban[len(iban)-4:]
				p.IbanLast4 = &last
			}
			label := strings.TrimSpace(a.Product)
			if label == "" {
				label = strings.TrimSpace(a.Name)
			}
			if label != "" {
				p.Label = &label
			}
			acc, err := q.UpsertAccount(ctx, p)
			if err != nil {
				return err
			}
			if profile.SyncAccount(a) {
				syncable = append(syncable, acc)
			}
		}
		if _, err := q.GetTrackedAccount(ctx, c.ID); errors.Is(err, pgx.ErrNoRows) && len(syncable) == 1 {
			return q.TrackAccount(ctx, store.TrackAccountParams{ID: syncable[0].ID, ConnectionID: c.ID})
		} else if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		return nil
	})
}

// accountCurrency : BoursoBank renvoie "XXX" (pas de devise) pour ses comptes ;
// on retient alors l'euro, seule devise de la version 1.
func accountCurrency(c string) string {
	if len(c) != 3 || c == "XXX" {
		return "EUR"
	}
	return strings.ToUpper(c)
}
