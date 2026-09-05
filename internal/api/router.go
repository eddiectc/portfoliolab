package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"codeberg.org/eddiectc/portfoliolab/internal/api/handlers"
	"codeberg.org/eddiectc/portfoliolab/internal/api/middleware"
	"codeberg.org/eddiectc/portfoliolab/internal/assets"
	"codeberg.org/eddiectc/portfoliolab/internal/config"
	"codeberg.org/eddiectc/portfoliolab/internal/data"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/account"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/allocation"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/analysis"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/comparison"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/efficientfrontier"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor/blackrock"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor/dimensional"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor/dws"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor/imgp"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor/vanguard"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/extractor/wisdomtree"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/hierarchicalriskparity"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/ibkrimport"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/marketcache"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/marketservice"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/modelportfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/position"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/seed"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/symbolmapping"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/symbols"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/trading212import"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/transaction"
	"codeberg.org/eddiectc/portfoliolab/internal/market"
	"codeberg.org/eddiectc/portfoliolab/internal/web"
)

// MarketDataFetcher bundles quote/FX/historical fetching with symbol-details
// fetching for every consumer wired in the router.
// *market.YahooFinanceFetcher satisfies it; tests may inject a stub to avoid
// real network calls.
type MarketDataFetcher interface {
	market.MarketDataFetcher
	market.SymbolDetailsFetcher
}

// RouterOption configures the router.
type RouterOption func(*routerConfig)

type routerConfig struct {
	templatesFS    fs.FS
	staticFS       fs.FS
	extractorCfg   config.ExtractorConfig
	marketFetcher  MarketDataFetcher
	seedSampleData bool
}

// WithTemplatesFS sets the templates FS for the router (defaults to the
// embedded templates).
func WithTemplatesFS(fsys fs.FS) RouterOption {
	return func(c *routerConfig) {
		c.templatesFS = fsys
	}
}

// WithStaticFS sets the static assets FS for the router (defaults to the
// embedded static files).
func WithStaticFS(fsys fs.FS) RouterOption {
	return func(c *routerConfig) {
		c.staticFS = fsys
	}
}

// WithExtractorConfig sets the extractor configuration.
func WithExtractorConfig(cfg config.ExtractorConfig) RouterOption {
	return func(c *routerConfig) {
		c.extractorCfg = cfg
	}
}

// WithMarketDataFetcher overrides the market data fetcher. When omitted,
// the real Yahoo Finance fetcher is used.
func WithMarketDataFetcher(f MarketDataFetcher) RouterOption {
	return func(c *routerConfig) {
		c.marketFetcher = f
	}
}

// WithoutSampleSeed disables the startup sample-data seed. Production
// enables the seed by default; tests opt out so they start from an empty
// database.
func WithoutSampleSeed() RouterOption {
	return func(c *routerConfig) {
		c.seedSampleData = false
	}
}

// Router builds and returns the application HTTP router and the MarketCache
// instance for lifecycle management (Start/Stop) in main.go.
func Router(db *sql.DB, logger *slog.Logger, opts ...RouterOption) (http.Handler, *marketcache.MarketCache) {
	cfg := &routerConfig{
		templatesFS:    assets.Templates,
		staticFS:       assets.Static,
		seedSampleData: true,
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
		// The status code is already sent; nothing more to do on failure.
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Extractor registry — WisdomTree extractor registered at startup.
	// New providers are added here.
	extractorReg := extractor.NewRegistry()
	// Registration can only fail on duplicate provider names, which is a
	// developer error for the fixed set registered here.
	mustRegister := func(e extractor.Extractor) {
		if err := extractorReg.Register(e); err != nil {
			panic(err)
		}
	}
	mustRegister(wisdomtree.NewExtractor())
	mustRegister(dws.NewExtractor())
	mustRegister(dimensional.NewExtractor())
	mustRegister(imgp.NewExtractor())
	mustRegister(blackrock.NewExtractor())
	vgExtractor := vanguard.NewExtractor()
	if cfg.extractorCfg.Vanguard.NavHistoryDays > 0 {
		vanguard.WithNavHistoryDays(cfg.extractorCfg.Vanguard.NavHistoryDays)(vgExtractor)
	}
	mustRegister(vgExtractor)
	extractorDispatcher := extractor.NewDispatcher(extractorReg)

	// Static files (embedded in the binary)
	r.Mount("/static", web.StaticHandler(cfg.staticFS))

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

	// Symbol CRUD (API)
	symbolMappingRepo := data.NewSymbolMappingRepository(db)
	var marketFetcher MarketDataFetcher = cfg.marketFetcher
	if marketFetcher == nil {
		marketFetcher = market.NewYahooFinanceFetcher(logger)
	}

	// Market data repository (stock quotes + FX rates)
	marketDataRepo := data.NewMarketDataRepository(db)

	// Symbol details (API enrichment)
	symbolDetailsRepo := data.NewSymbolDetailsRepository(db)
	symbolDetailsSvc := symbols.NewService(symbolDetailsRepo, marketFetcher)
	symbolDetailsSvc.WithExtractorDispatcher(extractorDispatcher)
	symbolDetailsSvc.WithDataSourceURLRepo(data.NewSymbolMappingDataSourceURLAdapter(symbolMappingRepo))
	symbolDetailsSvc.WithMarketDataRepo(marketDataRepo)

	symbolMappingSvc := symbolmapping.NewService(symbolMappingRepo,
		symbolmapping.WithMarketDataFetcher(marketFetcher),
		symbolmapping.WithSymbolDetailsFetcher(symbolDetailsSvc),
		symbolmapping.WithLogger(logger))
	symbolHandler := handlers.NewSymbolHandler(symbolMappingSvc, symbolDetailsSvc)
	symbolHandler.RegisterRoutes(r)

	// Transaction CRUD (API)
	transactionRepo := data.NewTransactionRepository(db)
	accountChecker := data.NewAccountChecker(accountRepo)
	symbolChecker := data.NewSymbolChecker(symbolMappingRepo)
	symbolCreator := data.NewSymbolCreator(symbolMappingSvc)

	// Portfolio currency checker (for FX conversion)
	portfolioCurrencyChecker := data.NewPortfolioCurrencyChecker(portfolioSvc)

	// Position service (used as LotChecker + PositionRecalculator for transactions)
	positionRepo := data.NewPositionRepository(db)
	accountLister := data.NewAccountLister(accountRepo)
	positionSvc := position.NewService(positionRepo, transactionRepo, accountChecker, portfolioChecker, accountLister, portfolioCurrencyChecker)
	// Wire market data service for enriching positions, FX rates, and refreshing.
	marketSvc := marketservice.New(marketFetcher, marketDataRepo)
	positionSvc.WithMarketDataService(marketSvc, logger)

	// Create market cache (needs discoverer for symbols and FX pairs).
	discoverer := data.NewMarketDataDiscoverer(symbolMappingRepo, transactionRepo)
	marketCache := marketcache.New(marketFetcher, marketDataRepo, discoverer, logger)
	marketCache.WithSymbolDetailsRefresh(symbolDetailsSvc)
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
	positionHandler := handlers.NewPositionHandler(positionSvc, portfolioSvc)
	positionHandler.RegisterRoutes(r)

	// Performance API
	performanceHandler := handlers.NewPerformanceHandler(positionSvc, marketSvc)
	performanceHandler.RegisterRoutes(r)

	// Market data API
	marketDataHandler := handlers.NewMarketDataHandler(marketCache)
	marketDataHandler.RegisterRoutes(r)

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

	// Model Portfolio API
	modelPortfolioRepo := data.NewModelPortfolioRepository(db)
	modelPortfolioSvc := modelportfolio.NewService(modelPortfolioRepo, symbolChecker, symbolCreator)
	modelPortfolioHandler := handlers.NewModelPortfolioHandler(modelPortfolioSvc)
	modelPortfolioHandler.RegisterRoutes(r)

	// Portfolio web pages (templates are embedded in the binary)
	renderer, err := web.NewRenderer(cfg.templatesFS, cfg.staticFS)
	if err != nil {
		logger.Error("failed to load embedded templates, web pages unavailable", "error", err)
	} else {
		portfolioWebHandler := handlers.NewPortfolioWebHandler(portfolioSvc, accountSvc, renderer)
		portfolioWebHandler.RegisterRoutes(r)

		// Account web pages
		accountWebHandler := handlers.NewAccountWebHandler(accountSvc, portfolioSvc, renderer)
		accountWebHandler.RegisterRoutes(r)

		// Symbol web pages
		symbolWebHandler := handlers.NewSymbolWebHandler(symbolMappingSvc, renderer)
		symbolWebHandler.RegisterRoutes(r)

		// Symbol details web pages
		symbolDetailsWebHandler := handlers.NewSymbolDetailsWebHandler(symbolMappingSvc, symbolDetailsSvc, marketFetcher, marketDataRepo, renderer)
		symbolDetailsWebHandler.RegisterRoutes(r)

		// Transaction web pages
		transactionWebHandler := handlers.NewTransactionWebHandler(transactionSvc, accountSvc, symbolMappingSvc, renderer)
		transactionWebHandler.RegisterRoutes(r)

		// Position web pages
		positionWebHandler := handlers.NewPositionWebHandler(positionHandler, positionSvc, accountSvc, portfolioSvc, marketCache, renderer)
		positionWebHandler.RegisterRoutes(r)

		// IBKR import web pages
		importWebHandler := handlers.NewImportWebHandler(importSvc, accountSvc, symbolMappingSvc, renderer)
		importWebHandler.RegisterRoutes(r)

		// Trading 212 import web pages
		t212WebHandler := handlers.NewTrading212ImportWebHandler(t212Svc, accountSvc, symbolMappingSvc, renderer)
		t212WebHandler.RegisterRoutes(r)

		// Performance web pages
		performanceWebHandler := handlers.NewPerformanceWebHandler(performanceHandler, portfolioSvc, marketCache, symbolMappingRepo, renderer)
		performanceWebHandler.RegisterRoutes(r)

		// Analysis service + API
		marketDataSymbolResolver := data.NewMarketDataSymbolResolver(symbolMappingRepo)
		analysisSvc := analysis.NewService(positionSvc, symbolDetailsSvc, marketSvc, accountLister, portfolioCurrencyChecker, marketDataSymbolResolver)
		analysisHandler := handlers.NewAnalysisHandler(analysisSvc)
		analysisHandler.RegisterRoutes(r)

		// Analysis web pages
		analysisWebHandler := handlers.NewAnalysisWebHandler(analysisHandler, portfolioSvc, renderer)
		analysisWebHandler.RegisterRoutes(r)

		// Allocation service + API
		targetAllocRepo := data.NewTargetAllocationRepository(db)
		allocSvc := allocation.NewService(positionSvc, accountLister, targetAllocRepo)
		allocSvc.WithLogger(logger)
		allocHandler := handlers.NewAllocationHandler(allocSvc)
		allocHandler.RegisterRoutes(r)

		// Allocation web pages
		allocWebHandler := handlers.NewAllocationWebHandler(allocHandler, portfolioSvc, symbolMappingSvc, allocSvc, modelPortfolioSvc, renderer)
		allocWebHandler.RegisterRoutes(r)

		// Comparison service + API
		portfolioNameResolver := data.NewPortfolioNameResolver(portfolioSvc)
		comparisonSvc := comparison.NewService(modelPortfolioSvc, positionSvc, marketSvc,
			marketDataSymbolResolver, symbolDetailsSvc,
			portfolioNameResolver, portfolioCurrencyChecker, allocSvc)
		comparisonSvc.WithLogger(logger)
		comparisonHandler := handlers.NewComparisonHandler(comparisonSvc)
		comparisonHandler.RegisterRoutes(r)

		// Comparison web pages
		comparisonWebHandler := handlers.NewComparisonWebHandler(comparisonHandler, portfolioSvc, modelPortfolioSvc, renderer)
		comparisonWebHandler.RegisterRoutes(r)

		// Shared optimization adapters (used by both Efficient Frontier and HRP)
		optSymbolLister := data.NewOptimizationSymbolLister(symbolMappingRepo)
		optPortfolioSymbols := data.NewOptimizationPortfolioSymbolSource(accountSvc, positionSvc)
		optModelPortfolio := data.NewOptimizationModelPortfolioSource(modelPortfolioSvc)
		optFxRates := data.NewOptimizationFxRateSource(marketSvc)

		// Efficient Frontier service + API
		efficientFrontierSvc := efficientfrontier.NewService(
			marketSvc,
			marketDataSymbolResolver,
			optSymbolLister,
			optPortfolioSymbols,
			optModelPortfolio,
			optFxRates,
		)
		efficientFrontierHandler := handlers.NewEfficientFrontierHandler(efficientFrontierSvc)
		efficientFrontierHandler.WithModelPortfolioCreator(modelPortfolioSvc)
		efficientFrontierHandler.RegisterRoutes(r)

		// Efficient Frontier web pages
		efficientFrontierWebHandler := handlers.NewEfficientFrontierWebHandler(
			efficientFrontierHandler,
			portfolioSvc,
			modelPortfolioSvc,
			renderer,
		)
		efficientFrontierWebHandler.WithModelPortfolioCreator(modelPortfolioSvc)
		efficientFrontierWebHandler.RegisterRoutes(r)

		// HRP service + API
		hrpSvc := hierarchicalriskparity.NewService(
			marketSvc,
			marketDataSymbolResolver,
			optSymbolLister,
			optPortfolioSymbols,
			optModelPortfolio,
			optFxRates,
		)
		hrpHandler := handlers.NewHrpHandler(hrpSvc)
		hrpHandler.WithModelPortfolioCreator(modelPortfolioSvc)
		hrpHandler.RegisterRoutes(r)

		// HRP web pages
		hrpWebHandler := handlers.NewHrpWebHandler(
			hrpHandler,
			portfolioSvc,
			modelPortfolioSvc,
			renderer,
		)
		hrpWebHandler.WithModelPortfolioCreator(modelPortfolioSvc)
		hrpWebHandler.RegisterRoutes(r)

		// Model Portfolio web pages
		modelPortfolioWebHandler := handlers.NewModelPortfolioWebHandler(modelPortfolioSvc, symbolMappingSvc, renderer)
		modelPortfolioWebHandler.RegisterRoutes(r)

		// Root redirect
		r.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "/portfolios", http.StatusSeeOther)
		})

		// Help pages
		r.Get("/help/fx", func(w http.ResponseWriter, r *http.Request) {
			if err := renderer.Render(w, "help/fx_conventions", web.PageData{Title: "FX Conventions"}); err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
		})
	}

	// Seed sample data on first startup (no-op when any portfolio already
	// exists). Runs before main.go starts the market cache workers, so the
	// recalculated positions are in place before the first refresh pass.
	// A failure is logged and non-fatal — the next startup retries.
	if cfg.seedSampleData {
		seedSvc := seed.NewService(data.NewSeedStore(db), positionSvc, logger)
		if err := seedSvc.SeedDemo(context.Background()); err != nil {
			logger.Error("sample data seed failed; will retry on next startup", "error", err)
		}
	}

	return r, marketCache
}
