package api

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/arch-portfolio-lab/portfoliolab/internal/api/handlers"
	"github.com/arch-portfolio-lab/portfoliolab/internal/api/middleware"
	"github.com/arch-portfolio-lab/portfoliolab/internal/data"
	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/account"
	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/portfolio"
	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/symbolmapping"
	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/transaction"
	"github.com/arch-portfolio-lab/portfoliolab/internal/market"
	"github.com/arch-portfolio-lab/portfoliolab/internal/web"
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

	// Static files
	r.Mount("/static", web.StaticHandler("internal/web/static"))

	// Portfolio CRUD (API)
	portfolioRepo := data.NewPortfolioRepository(db)
	portfolioSvc := portfolio.NewService(portfolioRepo)
	portfolioHandler := handlers.NewPortfolioHandler(portfolioSvc)
	portfolioHandler.RegisterRoutes(r)

	// Account CRUD (API)
	accountRepo := data.NewAccountRepository(db)
	portfolioChecker := data.NewPortfolioChecker(portfolioRepo)
	accountSvc := account.NewService(accountRepo, portfolioChecker)
	accountHandler := handlers.NewAccountHandler(accountSvc)
	accountHandler.RegisterRoutes(r)

	// Symbol mapping CRUD (API)
	symbolMappingRepo := data.NewSymbolMappingRepository(db)
	yahooFetcher := market.NewYahooFinanceFetcher(logger)
	symbolMappingSvc := symbolmapping.NewService(symbolMappingRepo, symbolmapping.WithQuoteFetcher(yahooFetcher))
	symbolMappingHandler := handlers.NewSymbolMappingHandler(symbolMappingSvc)
	symbolMappingHandler.RegisterRoutes(r)

	// Transaction CRUD (API)
	transactionRepo := data.NewTransactionRepository(db)
	accountChecker := data.NewAccountChecker(accountRepo)
	symbolChecker := data.NewSymbolChecker(symbolMappingRepo)
	symbolCreator := data.NewSymbolCreator(symbolMappingSvc)
	transactionSvc := transaction.NewService(transactionRepo, accountChecker, symbolChecker, symbolCreator)
	transactionHandler := handlers.NewTransactionHandler(transactionSvc)
	transactionHandler.RegisterRoutes(r)

	// Portfolio web pages
	renderer, err := web.NewRenderer("templates")
	if err != nil {
		logger.Warn("failed to load templates, web pages unavailable", "error", err)
		// Fall back: check if templates dir exists relative to cwd
		if _, statErr := os.Stat("templates"); statErr != nil {
			logger.Warn("templates directory not found", "path", "templates")
		}
	} else {
		portfolioWebHandler := handlers.NewPortfolioWebHandler(portfolioSvc, accountSvc, renderer)
		portfolioWebHandler.RegisterRoutes(r)

		// Account web pages
		accountWebHandler := handlers.NewAccountWebHandler(accountSvc, portfolioSvc, renderer)
		accountWebHandler.RegisterRoutes(r)

		// Symbol mapping web pages
		symbolMappingWebHandler := handlers.NewSymbolMappingWebHandler(symbolMappingSvc, renderer)
		symbolMappingWebHandler.RegisterRoutes(r)

		// Transaction web pages
		transactionWebHandler := handlers.NewTransactionWebHandler(transactionSvc, accountSvc, symbolMappingSvc, renderer)
		transactionWebHandler.RegisterRoutes(r)

		// Root redirect
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/portfolios", http.StatusSeeOther)
		})
	}

	return r
}
