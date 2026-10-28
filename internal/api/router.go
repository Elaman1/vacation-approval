package api

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5/pgxpool"

	"vacation-approval/internal/api/docs"
	"vacation-approval/internal/api/handler"
	"vacation-approval/internal/api/middleware"
)

func NewRouter(db *pgxpool.Pool, verifier *oidc.IDTokenVerifier, vacationHandler *handler.VacationHandler, managerHandler *handler.ManagerHandler) *http.ServeMux {
	mux := http.NewServeMux()
	docs.Register(mux)
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
	return mux
}
