package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/timotheebenchergui/arrondi/api/internal/store"
)

type ctxKey struct{}

// reqInfo est posé par accessLog et complété par authenticate, pour journaliser l'utilisateur.
type reqInfo struct{ userID string }

type infoKey struct{}

// userFrom renvoie l'utilisateur authentifié (posé par authenticate).
func userFrom(ctx context.Context) store.User {
	u, _ := ctx.Value(ctxKey{}).(store.User)
	return u
}

func writeProblem(w http.ResponseWriter, status int, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "about:blank", "title": http.StatusText(status), "status": status, "detail": detail,
	})
}

// securityHeaders : section 10.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

const maxBody = 64 << 10 // 64 Ko

func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		next.ServeHTTP(w, r)
	})
}

// requireAjax : toute requête qui modifie des données porte X-Requested-With: epargne
// et, si elle a un corps, Content-Type: application/json (section 10).
func requireAjax(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if r.Header.Get("X-Requested-With") != "epargne" {
				writeProblem(w, http.StatusBadRequest, "en-tête X-Requested-With manquant")
				return
			}
			if (r.ContentLength > 0 || r.Header.Get("Transfer-Encoding") != "") &&
				!strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				writeProblem(w, http.StatusBadRequest, "Content-Type: application/json attendu")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// authenticate résout l'utilisateur à partir du JWT Cloudflare Access.
// 401 sans JWT valide, 403 si l'email est inconnu ou le compte désactivé.
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		email, err := s.Verifier.Email(r)
		if err != nil {
			writeProblem(w, http.StatusUnauthorized, "authentification Cloudflare Access requise")
			return
		}
		u, err := s.App.Q.GetActiveUserByEmail(r.Context(), email)
		if errors.Is(err, pgx.ErrNoRows) {
			writeProblem(w, http.StatusForbidden, "compte inconnu ou désactivé")
			return
		}
		if err != nil {
			writeProblem(w, http.StatusInternalServerError, "erreur interne")
			return
		}
		if info, ok := r.Context().Value(infoKey{}).(*reqInfo); ok {
			info.userID = u.ID.String()
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	})
}

// rateLimiter : 60 requêtes par minute et par utilisateur (fenêtre fixe).
type rateLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	now    func() time.Time
	counts map[string]*bucket
}

type bucket struct {
	start time.Time
	n     int
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{limit: limit, window: window, now: time.Now, counts: map[string]*bucket{}}
}

func (l *rateLimiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if len(l.counts) > 1000 { // nettoyage des fenêtres terminées
		for k, b := range l.counts {
			if now.Sub(b.start) >= l.window {
				delete(l.counts, k)
			}
		}
	}
	b := l.counts[key]
	if b == nil || now.Sub(b.start) >= l.window {
		l.counts[key] = &bucket{start: now, n: 1}
		return true
	}
	b.n++
	return b.n <= l.limit
}

func (s *Server) rateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.limiter.allow(userFrom(r.Context()).ID.String()) {
			w.Header().Set("Retry-After", "60")
			writeProblem(w, http.StatusTooManyRequests, "trop de requêtes, réessayez dans une minute")
			return
		}
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) { r.status = code; r.ResponseWriter.WriteHeader(code) }

// accessLog : JSON, sans paramètres de requête (le callback porte code et state),
// ni email, ni montant (section 10).
func accessLog(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		info := &reqInfo{}
		next.ServeHTTP(rec, r.WithContext(context.WithValue(r.Context(), infoKey{}, info)))
		attrs := []any{"method", r.Method, "path", r.URL.Path, "status", rec.status, "ms", time.Since(start).Milliseconds()}
		if info.userID != "" {
			attrs = append(attrs, "user_id", info.userID)
		}
		log.Info("http", attrs...)
	})
}
