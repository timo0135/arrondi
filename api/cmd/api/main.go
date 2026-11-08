// Commande api : serveur HTTP d'Arrondi et synchronisation planifiée (06:30).
//
// Variables (cahier des charges, section 12) :
//
//	DATABASE_URL            adresse PostgreSQL (secret)
//	MASTER_KEY_FILE         clé maître de 32 octets (défaut /run/secrets/master_key)
//	PUBLIC_BASE_URL         https://arrondi.timotheebenchergui.fr
//	CF_ACCESS_TEAM_DOMAIN   <équipe>.cloudflareaccess.com
//	CF_ACCESS_AUD           audience de l'application Access
//	SYNC_CRON               défaut "CRON_TZ=Europe/Paris 30 6 * * *"
//	LOG_LEVEL               debug, info (défaut), warn, error
//	LISTEN_ADDR             défaut :8080
//	BOOTSTRAP_ADMIN_EMAIL   crée l'administrateur si la base est vide
//	EB_BASE_URL             défaut https://api.enablebanking.com
//
// Développement local uniquement : APP_ENV=dev et DEV_USER_EMAIL=<email> remplacent
// Cloudflare Access par une identité fixe.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robfig/cron/v3"

	"github.com/timotheebenchergui/arrondi/api/internal/app"
	"github.com/timotheebenchergui/arrondi/api/internal/auth"
	"github.com/timotheebenchergui/arrondi/api/internal/enablebanking"
	"github.com/timotheebenchergui/arrondi/api/internal/httpapi"
	"github.com/timotheebenchergui/arrondi/api/internal/keybox"
)

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel(env("LOG_LEVEL", "info"))}))
	if err := run(log); err != nil {
		log.Error("fatal", "error", err.Error())
		os.Exit(1)
	}
}

func logLevel(s string) slog.Level {
	var l slog.Level
	if err := l.UnmarshalText([]byte(s)); err != nil {
		return slog.LevelInfo
	}
	return l
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dbURL := env("DATABASE_URL", "")
	baseURL := strings.TrimRight(env("PUBLIC_BASE_URL", ""), "/")
	if dbURL == "" || baseURL == "" {
		return errors.New("DATABASE_URL et PUBLIC_BASE_URL sont obligatoires")
	}

	box, err := keybox.LoadFile(env("MASTER_KEY_FILE", "/run/secrets/master_key"))
	if err != nil {
		return err
	}

	var verifier auth.Verifier
	if env("APP_ENV", "") == "dev" {
		email := env("DEV_USER_EMAIL", "")
		if email == "" {
			return errors.New("APP_ENV=dev exige DEV_USER_EMAIL")
		}
		log.Warn("dev_auth_enabled", "detail", "Cloudflare Access désactivé : ne jamais utiliser en production")
		verifier = auth.DevVerifier{EmailAddr: email}
	} else if verifier, err = auth.NewAccessVerifier(env("CF_ACCESS_TEAM_DOMAIN", ""), env("CF_ACCESS_AUD", "")); err != nil {
		return err
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("base de données : %w", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("base de données injoignable : %w", err)
	}

	a := app.New(pool, box, enablebanking.New(env("EB_BASE_URL", "")), log)
	if err := a.Bootstrap(ctx, env("BOOTSTRAP_ADMIN_EMAIL", "")); err != nil {
		return err
	}

	// Synchronisation quotidienne (section 9.3).
	sched := cron.New()
	spec := env("SYNC_CRON", "CRON_TZ=Europe/Paris 30 6 * * *")
	if _, err := sched.AddFunc(spec, func() {
		jobCtx, cancel := context.WithTimeout(ctx, 2*time.Hour)
		defer cancel()
		a.RunDaily(jobCtx)
	}); err != nil {
		return fmt.Errorf("SYNC_CRON invalide : %w", err)
	}
	sched.Start()
	defer func() { <-sched.Stop().Done() }()

	srv := &http.Server{
		Addr:              env("LISTEN_ADDR", ":8080"),
		Handler:           (&httpapi.Server{App: a, Verifier: verifier, PublicBaseURL: baseURL, Log: log}).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("api_started", "addr", srv.Addr, "sync_cron", spec)

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("api_stopping")
	shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutCtx)
}
