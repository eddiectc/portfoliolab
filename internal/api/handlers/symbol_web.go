package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/symbolmapping"
	"codeberg.org/eddiectc/portfoliolab/internal/web"
)

// symbolMappingFormPageData is the shared data struct for the symbol mapping form template.
type symbolMappingFormPageData struct {
	web.PageData
	InternalSymbol   string
	MarketDataSymbol string
	IsBenchmark      bool
	BrokerSymbols    []symbolmapping.BrokerSymbolRequest
	PreviewName      string
	PreviewExchange  string
	PreviewPrice     string
	PreviewCurrency  string
	Action           string
	SubmitText       string
	CancelHref       string
}

// newSymbolMappingFormPageData creates a symbolMappingFormPageData with common defaults.
func newSymbolMappingFormPageData(pd web.PageData, action, submitText, cancelHref string) *symbolMappingFormPageData {
	return &symbolMappingFormPageData{
		PageData:       pd,
		BrokerSymbols:  []symbolmapping.BrokerSymbolRequest{},
		Action:         action,
		SubmitText:     submitText,
		CancelHref:     cancelHref,
	}
}

// SymbolWebHandler handles server-rendered symbol pages.
type SymbolWebHandler struct {
	service  *symbolmapping.Service
	renderer *web.Renderer
}

// NewSymbolWebHandler creates a new symbol web handler.
func NewSymbolWebHandler(service *symbolmapping.Service, renderer *web.Renderer) *SymbolWebHandler {
	return &SymbolWebHandler{
		service:  service,
		renderer: renderer,
	}
}

// RegisterRoutes mounts web symbol routes on the given router.
// Note: more specific routes (with sub-paths) must be registered before catch-all routes.
func (h *SymbolWebHandler) RegisterRoutes(r *chi.Mux) {
	// Specific routes first
	r.Post("/symbols/{id}/delete", h.HandleDeletePage)
	r.Post("/symbols/{id}/edit", h.HandleUpdatePage)
	r.Get("/symbols/{id}/edit", h.HandleEditPage)
	r.Get("/symbols/new", h.HandleNewPage)
	// Catch-all routes last
	r.Post("/symbols", h.HandleCreatePage)
	r.Get("/symbols", h.HandleListPage)
}

// HandleListPage renders GET /symbols.
func (h *SymbolWebHandler) HandleListPage(w http.ResponseWriter, r *http.Request) {
	mappings, err := h.service.ListAll(r.Context())
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	data := struct {
		web.PageData
		Mappings []symbolmapping.SymbolMapping
	}{
		PageData: web.PageData{
			Title: "Symbols",
			Flash: getFlash(w, r),
		},
		Mappings: mappings,
	}

	if data.Mappings == nil {
		data.Mappings = []symbolmapping.SymbolMapping{}
	}

	if err := h.renderer.Render(w, "symbol/list", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleNewPage renders GET /symbols/new.
func (h *SymbolWebHandler) HandleNewPage(w http.ResponseWriter, r *http.Request) {
	data := newSymbolMappingFormPageData(web.PageData{
		Title: "New Symbol Mapping",
	}, "/symbols", "Create Mapping", "/symbols")

	if err := h.renderer.Render(w, "symbol/form", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// parseBrokerSymbols extracts broker symbol pairs from form values.
// Form fields are named "broker_name" and "broker_symbol" (repeated for each row).
func parseBrokerSymbols(r *http.Request) []symbolmapping.BrokerSymbolRequest {
	names := r.Form["broker_name"]
	symbols := r.Form["broker_symbol"]

	count := len(names)
	if len(symbols) < count {
		count = len(symbols)
	}

	var result []symbolmapping.BrokerSymbolRequest
	for i := 0; i < count; i++ {
		name := strings.TrimSpace(names[i])
		symbol := strings.TrimSpace(symbols[i])
		if name != "" && symbol != "" {
			result = append(result, symbolmapping.BrokerSymbolRequest{
				BrokerName:   name,
				BrokerSymbol: symbol,
			})
		}
	}
	return result
}

// HandleCreatePage handles POST /symbols (form submission).
func (h *SymbolWebHandler) HandleCreatePage(w http.ResponseWriter, r *http.Request) {
	internalSymbol := r.FormValue("internal_symbol")
	marketDataSymbol := r.FormValue("market_data_symbol")
	isBenchmark := r.FormValue("is_benchmark") == "on"
	brokerSymbols := parseBrokerSymbols(r)

	req := symbolmapping.CreateRequest{
		InternalSymbol:   internalSymbol,
		MarketDataSymbol: marketDataSymbol,
		IsBenchmark:      isBenchmark,
		BrokerSymbols:    brokerSymbols,
	}

	sm, err := h.service.Create(r.Context(), req)
	if err != nil {
		data := newSymbolMappingFormPageData(web.PageData{
			Title: "New Symbol Mapping",
			Error: symbolMappingUserFriendlyError(err),
		}, "/symbols", "Create Mapping", "/symbols")
		data.InternalSymbol = internalSymbol
		data.MarketDataSymbol = marketDataSymbol
		data.IsBenchmark = isBenchmark
		data.BrokerSymbols = brokerSymbols

		if renderErr := h.renderer.Render(w, "symbol/form", data); renderErr != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		return
	}

	setFlash(w, "Symbol mapping \""+sm.InternalSymbol+"\" created successfully")
	http.Redirect(w, r, "/symbols", http.StatusSeeOther)
}

// HandleEditPage renders GET /symbols/{id}/edit.
func (h *SymbolWebHandler) HandleEditPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	sm, err := h.service.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	editAction := "/symbols/" + strconv.FormatInt(id, 10) + "/edit"
	cancelHref := "/symbols"

	// Convert existing broker symbols to form requests
	var brokerReqs []symbolmapping.BrokerSymbolRequest
	for _, bs := range sm.BrokerSymbols {
		brokerReqs = append(brokerReqs, symbolmapping.BrokerSymbolRequest{
			BrokerName:   bs.BrokerName,
			BrokerSymbol: bs.BrokerSymbol,
		})
	}

	data := newSymbolMappingFormPageData(web.PageData{
		Title: "Edit Symbol Mapping",
	}, editAction, "Save Changes", cancelHref)
	data.InternalSymbol = sm.InternalSymbol
	data.MarketDataSymbol = sm.MarketDataSymbol
	data.IsBenchmark = sm.IsBenchmark
	data.BrokerSymbols = brokerReqs

	if err := h.renderer.Render(w, "symbol/form", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleUpdatePage handles POST /symbols/{id}/edit.
func (h *SymbolWebHandler) HandleUpdatePage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	internalSymbol := r.FormValue("internal_symbol")
	marketDataSymbol := r.FormValue("market_data_symbol")
	isBenchmark := r.FormValue("is_benchmark") == "on"

	req := symbolmapping.UpdateRequest{}

	// Only include fields that were actually changed
	current, err := h.service.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if internalSymbol != current.InternalSymbol {
		req.InternalSymbol = &internalSymbol
	}
	if marketDataSymbol != current.MarketDataSymbol {
		req.MarketDataSymbol = &marketDataSymbol
	}
	if isBenchmark != current.IsBenchmark {
		req.IsBenchmark = &isBenchmark
	}

	_, err = h.service.Update(r.Context(), id, req)
	if err != nil {
		editAction := "/symbols/" + strconv.FormatInt(id, 10) + "/edit"
		cancelHref := "/symbols"

		// Re-parse broker symbols for form re-render
		brokerSymbols := parseBrokerSymbols(r)

		data := newSymbolMappingFormPageData(web.PageData{
			Title: "Edit Symbol Mapping",
			Error: symbolMappingUserFriendlyError(err),
		}, editAction, "Save Changes", cancelHref)
		data.InternalSymbol = internalSymbol
		data.MarketDataSymbol = marketDataSymbol
		data.IsBenchmark = isBenchmark
		data.BrokerSymbols = brokerSymbols

		if renderErr := h.renderer.Render(w, "symbol/form", data); renderErr != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		return
	}

	setFlash(w, "Symbol mapping updated successfully")
	http.Redirect(w, r, "/symbols", http.StatusSeeOther)
}

// HandleDeletePage handles POST /symbols/{id}/delete.
func (h *SymbolWebHandler) HandleDeletePage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		if errors.Is(err, symbolmapping.ErrInUse) {
			http.Redirect(w, r, "/symbols?error=in_use", http.StatusSeeOther)
			return
		}
		http.NotFound(w, r)
		return
	}

	setFlash(w, "Symbol mapping deleted successfully")
	http.Redirect(w, r, "/symbols", http.StatusSeeOther)
}

// symbolMappingUserFriendlyError returns a user-friendly message from a symbol mapping service error.
func symbolMappingUserFriendlyError(err error) string {
	if errors.Is(err, symbolmapping.ErrInvalidSymbol) {
		return "Invalid symbol: must be 1-20 characters"
	}
	if errors.Is(err, symbolmapping.ErrInternalSymbolExists) {
		return "A symbol mapping with this internal symbol already exists"
	}
	if errors.Is(err, symbolmapping.ErrBrokerSymbolExists) {
		return "This broker symbol is already mapped to a different internal symbol"
	}
	if errors.Is(err, symbolmapping.ErrInUse) {
		return "Cannot delete: this symbol mapping is referenced by transactions"
	}
	return "An error occurred. Please try again."
}
