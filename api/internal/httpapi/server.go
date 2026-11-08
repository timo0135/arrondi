// Package httpapi expose l'API REST d'Arrondi (cahier des charges, section 7)
// avec huma, qui génère le contrat OpenAPI (/api/v1/openapi.json).
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/timotheebenchergui/arrondi/api/internal/app"
	"github.com/timotheebenchergui/arrondi/api/internal/auth"
)

const prefix = "/api/v1"

// Server porte les dépendances des routes.
type Server struct {
	App           *app.App
	Verifier      auth.Verifier
	PublicBaseURL string // https://arrondi.timotheebenchergui.fr (D-14)
	Log           *slog.Logger

	limiter *rateLimiter
}

func (s *Server) now() time.Time { return s.App.Now() }

// RedirectURL est l'URL de retour déclarée dans la console Enable Banking.
func (s *Server) RedirectURL() string { return s.PublicBaseURL + prefix + "/bank/callback" }

// Handler assemble les routes et les middlewares.
func (s *Server) Handler() http.Handler {
	if s.limiter == nil {
		s.limiter = newRateLimiter(60, time.Minute)
	}
	apiMux := http.NewServeMux()
	config := huma.DefaultConfig("Arrondi API", "1.0.0")
	config.Info.Description = "API de l'application Arrondi. Montants en centimes, dates AAAA-MM-JJ, erreurs RFC 9457."
	api := humago.NewWithPrefix(apiMux, prefix, config)

	s.registerMe(api)
	s.registerSavings(api)
	s.registerBank(api)
	s.registerAdmin(api)
	apiMux.HandleFunc("GET "+prefix+"/bank/callback", s.bankCallback)

	protected := s.authenticate(s.rateLimit(requireAjax(apiMux)))

	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", s.healthz) // réseau Docker uniquement, non relayé par nginx
	root.Handle("/", protected)
	return accessLog(s.Log, securityHeaders(limitBody(root)))
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.App.Pool.Ping(ctx); err != nil {
		writeProblem(w, http.StatusServiceUnavailable, "base de données injoignable")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// --- helpers communs ---

var paris = func() *time.Location {
	l, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		panic(err)
	}
	return l
}()

func today(now time.Time) time.Time {
	y, m, d := now.In(paris).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func firstOfMonth(day time.Time) time.Time {
	return time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC)
}

const dateFmt = "2006-01-02"

func internalError(log *slog.Logger, op string, err error) error {
	log.Error("internal_error", "op", op, "error", err.Error())
	return huma.Error500InternalServerError("erreur interne")
}
