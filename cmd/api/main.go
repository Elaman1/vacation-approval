package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5/pgxpool"

	"vacation-approval/internal/api"
	"vacation-approval/internal/api/handler"
	"vacation-approval/internal/core/vacation"
	"vacation-approval/internal/integration/camunda"
	"vacation-approval/internal/integration/postgres"
)

func requiredEnv(name string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		log.Fatalf("%s must be set", name)
	}
	return value
}

func main() {
	databaseURL := requiredEnv("DATABASE_URL")
	camundaURL := requiredEnv("CAMUNDA_URL")
	issuer := requiredEnv("OIDC_ISSUER")
	jwksURL := requiredEnv("OIDC_JWKS_URL")
	audience := requiredEnv("OIDC_AUDIENCE")

	keySet := oidc.NewRemoteKeySet(context.Background(), jwksURL)
	verifier := oidc.NewVerifier(issuer, keySet, &oidc.Config{
		ClientID:             audience,
		SupportedSigningAlgs: []string{"RS256"},
	})

	db, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		log.Fatalf("create database pool: %v", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	err = db.Ping(ctx)
	cancel()
	if err != nil {
		log.Fatalf("connect to PostgreSQL: %v", err)
	}

	employeeRepo := postgres.NewEmployeeRepository(db)
	vacationRepo := postgres.NewVacationRepository(db)
	service := vacation.NewService(employeeRepo, vacationRepo)
	camundaClient := camunda.NewClient(camundaURL, "api")
	vacationHandler := handler.NewVacationHandler(service, camundaClient)
	managerHandler := handler.NewManagerHandler(vacationRepo)

	mux := api.NewRouter(db, verifier, vacationHandler, managerHandler)

	addr := os.Getenv("API_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("API listening on %s with Keycloak JWT verification", addr)
	runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)
	go func() { serverErr <- server.ListenAndServe() }()

	select {
	case err := <-serverErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server: %v", err)
		}
	case <-runCtx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
			_ = server.Close()
		}
		if err := <-serverErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("HTTP server: %v", err)
		}
	}
	log.Println("API stopped")
}
