package api

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/arch-portfolio-lab/portfoliolab/internal/api/handlers"
	"github.com/arch-portfolio-lab/portfoliolab/internal/api/middleware"
	"github.com/arch-portfolio-lab/portfoliolab/internal/data"
	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/portfolio"
)

// Router builds and returns the application HTTP router.
func Router(db *sql.DB, logger *slog.Logger) http.Handler {
	r := chi.NewRouter()

	// Standard middleware
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(middleware.LoggingMiddleware(logger))
	r.Use(chimiddleware.Recoverer)

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Portfolio CRUD
	portfolioRepo := data.NewPortfolioRepository(db)
	portfolioSvc := portfolio.NewService(portfolioRepo)
	portfolioHandler := handlers.NewPortfolioHandler(portfolioSvc)
	portfolioHandler.RegisterRoutes(r)

	return r
}
