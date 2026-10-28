package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5/pgxpool"

	apidocs "vacation-approval/internal/api/docs"
	"vacation-approval/internal/api/handler"
	"vacation-approval/internal/api/middleware"
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

	// Static public issuer + separately reachable JWKS endpoint. This is needed
	// for local Docker networking: browsers see localhost:8082, containers see keycloak:8080.
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

	mux := http.NewServeMux()
	apidocs.Register(mux)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			log.Printf("database ping failed: %v", err)
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})

	mux.Handle("POST /vacations", middleware.JWTAuth(verifier, "employee", http.HandlerFunc(vacationHandler.Create)))
	mux.Handle("GET /vacations", middleware.JWTAuth(verifier, "employee", http.HandlerFunc(vacationHandler.List)))
	mux.Handle("GET /manager/pending-vacations", middleware.JWTAuth(verifier, "manager", http.HandlerFunc(managerHandler.Pending)))

	addr := os.Getenv("API_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080" // Safe local default; Compose explicitly sets API_ADDR=:8080
	}
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("API listening on %s with Keycloak JWT verification", addr)
	log.Fatal(server.ListenAndServe())
}
