package api

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"codeberg.org/eddiectc/portfoliolab/internal/api/handlers"
	"codeberg.org/eddiectc/portfoliolab/internal/api/middleware"
	"codeberg.org/eddiectc/portfoliolab/internal/data"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/account"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/ibkrimport"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/marketservice"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/marketcache"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/position"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/trading212import"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/symbolmapping"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/transaction"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"codeberg.org/eddiectc/portfoliolab/internal/web"
)

// RouterOption configures the router.
type RouterOption func(*routerConfig)

type routerConfig struct {
	templatesDir string
}

// WithTemplatesDir sets the templates directory for the router.
func WithTemplatesDir(dir string) RouterOption {
	return func(c *routerConfig) {
		c.templatesDir = dir
	}
}

// Router builds and returns the application HTTP router and the MarketCache
// instance for lifecycle management (Start/Stop) in main.go.
func Router(db *sql.DB, logger *slog.Logger, opts ...RouterOption) (http.Handler, *marketcache.MarketCache) {
	cfg := &routerConfig{
		templatesDir: "templates",
	}
	for _, opt := range opts {
		opt(cfg)
	}
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
	symbolMappingSvc := symbolmapping.NewService(symbolMappingRepo, symbolmapping.WithMarketDataFetcher(yahooFetcher))
	symbolMappingHandler := handlers.NewSymbolMappingHandler(symbolMappingSvc)
	symbolMappingHandler.RegisterRoutes(r)

	// Transaction CRUD (API)
	transactionRepo := data.NewTransactionRepository(db)
	accountChecker := data.NewAccountChecker(accountRepo)
	symbolChecker := data.NewSymbolChecker(symbolMappingRepo)
	symbolCreator := data.NewSymbolCreator(symbolMappingSvc)

	// Market data repository (stock quotes + FX rates)
	marketDataRepo := data.NewMarketDataRepository(db)

	// Portfolio currency checker (for FX conversion)
	portfolioCurrencyChecker := data.NewPortfolioCurrencyChecker(portfolioRepo)

	// Position service (used as LotChecker + PositionRecalculator for transactions)
	positionRepo := data.NewPositionRepository(db)
	accountLister := data.NewAccountLister(accountRepo)
	positionSvc := position.NewService(positionRepo, transactionRepo, accountChecker, portfolioChecker, accountLister, portfolioCurrencyChecker)
	// Wire market data service for enriching positions, FX rates, and refreshing.
	marketSvc := marketservice.New(yahooFetcher, marketDataRepo)
	positionSvc.WithMarketDataService(marketSvc, logger)

	// Create market cache (needs position service as SymbolDiscoverer).
	marketCache := marketcache.New(yahooFetcher, marketDataRepo, positionSvc, logger)
	// Wire market cache into position service for scheduling.
	positionSvc.WithMarketCache(marketCache)

	transactionSvc := transaction.NewService(transactionRepo, accountChecker, symbolChecker, symbolCreator, positionSvc, positionSvc)
	// Wire market cache scheduling into transaction mutations.
	transactionSvc.WithCacheScheduler(positionSvc)
	transactionSvc.WithEarliestDateFinder(transactionRepo)
	transactionSvc.WithAccountPortfolioFinder(accountRepo)
	transactionSvc.WithPortfolioCurrencyResolver(portfolioRepo)
	transactionHandler := handlers.NewTransactionHandler(transactionSvc)
	transactionHandler.RegisterRoutes(r)

	// Position API
	positionHandler := handlers.NewPositionHandler(positionSvc)
	positionHandler.RegisterRoutes(r)

	// Performance API
	performanceHandler := handlers.NewPerformanceHandler(positionSvc)
	performanceHandler.RegisterRoutes(r)

	// IBKR Flex XML Import (API)
	symbolResolver := data.NewSymbolResolver(symbolMappingRepo)
	brokerSymbolAdder := data.NewBrokerSymbolAdder(symbolMappingRepo, symbolMappingSvc)
	importSvc := ibkrimport.NewService(symbolResolver, transactionRepo, transactionRepo, accountChecker, symbolCreator, brokerSymbolAdder,
		ibkrimport.WithPositionRecalculator(positionSvc), ibkrimport.WithLogger(logger))
	importHandler := handlers.NewImportHandler(importSvc, symbolMappingSvc)
	importHandler.RegisterRoutes(r)

	// Trading 212 CSV Import (API)
	t212Svc := trading212import.NewService(symbolResolver, transactionRepo, transactionRepo, accountChecker, symbolCreator, brokerSymbolAdder,
		trading212import.WithPositionRecalculator(positionSvc), trading212import.WithLogger(logger))
	t212Handler := handlers.NewTrading212ImportHandler(t212Svc, symbolMappingSvc)
	t212Handler.RegisterRoutes(r)

	// Portfolio web pages
	renderer, err := web.NewRenderer(cfg.templatesDir)
	if err != nil {
		logger.Warn("failed to load templates, web pages unavailable", "error", err)
		// Fall back: check if templates dir exists relative to cwd
		if _, statErr := os.Stat(cfg.templatesDir); statErr != nil {
			logger.Warn("templates directory not found", "path", cfg.templatesDir)
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

		// Position web pages
		positionWebHandler := handlers.NewPositionWebHandler(positionSvc, accountSvc, portfolioSvc, renderer)
		positionWebHandler.RegisterRoutes(r)

		// IBKR import web pages
		importWebHandler := handlers.NewImportWebHandler(importSvc, accountSvc, symbolMappingSvc, renderer)
		importWebHandler.RegisterRoutes(r)

		// Trading 212 import web pages
		t212WebHandler := handlers.NewTrading212ImportWebHandler(t212Svc, accountSvc, symbolMappingSvc, renderer)
		t212WebHandler.RegisterRoutes(r)

		// Performance web pages
		performanceWebHandler := handlers.NewPerformanceWebHandler(positionSvc, portfolioSvc, renderer)
		performanceWebHandler.RegisterRoutes(r)

		// Root redirect
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/portfolios", http.StatusSeeOther)
		})
	}

	return r, marketCache
}
