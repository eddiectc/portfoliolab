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

// SymbolMappingWebHandler handles server-rendered symbol mapping pages.
type SymbolMappingWebHandler struct {
	service  *symbolmapping.Service
	renderer *web.Renderer
}

// NewSymbolMappingWebHandler creates a new symbol mapping web handler.
func NewSymbolMappingWebHandler(service *symbolmapping.Service, renderer *web.Renderer) *SymbolMappingWebHandler {
	return &SymbolMappingWebHandler{
		service:  service,
		renderer: renderer,
	}
}

// RegisterRoutes mounts web symbol mapping routes on the given router.
// Note: more specific routes (with sub-paths) must be registered before catch-all routes.
func (h *SymbolMappingWebHandler) RegisterRoutes(r *chi.Mux) {
	// Specific routes first
	r.Post("/symbol-mappings/{id}/delete", h.HandleDeletePage)
	r.Post("/symbol-mappings/{id}/edit", h.HandleUpdatePage)
	r.Get("/symbol-mappings/{id}/edit", h.HandleEditPage)
	r.Get("/symbol-mappings/new", h.HandleNewPage)
	// Catch-all routes last
	r.Post("/symbol-mappings", h.HandleCreatePage)
	r.Get("/symbol-mappings", h.HandleListPage)
}

// HandleListPage renders GET /symbol-mappings.
func (h *SymbolMappingWebHandler) HandleListPage(w http.ResponseWriter, r *http.Request) {
	mappings, err := h.service.List(r.Context(), 0, 0)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	data := struct {
		web.PageData
		Mappings []symbolmapping.SymbolMapping
	}{
		PageData: web.PageData{
			Title: "Symbol Mappings",
			Flash: getFlash(w, r),
		},
		Mappings: mappings,
	}

	if data.Mappings == nil {
		data.Mappings = []symbolmapping.SymbolMapping{}
	}

	if err := h.renderer.Render(w, "symbol_mapping/list", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleNewPage renders GET /symbol-mappings/new.
func (h *SymbolMappingWebHandler) HandleNewPage(w http.ResponseWriter, r *http.Request) {
	data := newSymbolMappingFormPageData(web.PageData{
		Title: "New Symbol Mapping",
	}, "/symbol-mappings", "Create Mapping", "/symbol-mappings")

	if err := h.renderer.Render(w, "symbol_mapping/form", data); err != nil {
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

// HandleCreatePage handles POST /symbol-mappings (form submission).
func (h *SymbolMappingWebHandler) HandleCreatePage(w http.ResponseWriter, r *http.Request) {
	internalSymbol := r.FormValue("internal_symbol")
	marketDataSymbol := r.FormValue("market_data_symbol")
	brokerSymbols := parseBrokerSymbols(r)

	req := symbolmapping.CreateRequest{
		InternalSymbol:   internalSymbol,
		MarketDataSymbol: marketDataSymbol,
		BrokerSymbols:    brokerSymbols,
	}

	sm, err := h.service.Create(r.Context(), req)
	if err != nil {
		data := newSymbolMappingFormPageData(web.PageData{
			Title: "New Symbol Mapping",
			Error: symbolMappingUserFriendlyError(err),
		}, "/symbol-mappings", "Create Mapping", "/symbol-mappings")
		data.InternalSymbol = internalSymbol
		data.MarketDataSymbol = marketDataSymbol
		data.BrokerSymbols = brokerSymbols

		if renderErr := h.renderer.Render(w, "symbol_mapping/form", data); renderErr != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		return
	}

	setFlash(w, "Symbol mapping \""+sm.InternalSymbol+"\" created successfully")
	http.Redirect(w, r, "/symbol-mappings", http.StatusSeeOther)
}

// HandleEditPage renders GET /symbol-mappings/{id}/edit.
func (h *SymbolMappingWebHandler) HandleEditPage(w http.ResponseWriter, r *http.Request) {
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

	editAction := "/symbol-mappings/" + strconv.FormatInt(id, 10) + "/edit"
	cancelHref := "/symbol-mappings"

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
	data.BrokerSymbols = brokerReqs

	if err := h.renderer.Render(w, "symbol_mapping/form", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleUpdatePage handles POST /symbol-mappings/{id}/edit.
func (h *SymbolMappingWebHandler) HandleUpdatePage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	internalSymbol := r.FormValue("internal_symbol")
	marketDataSymbol := r.FormValue("market_data_symbol")

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

	_, err = h.service.Update(r.Context(), id, req)
	if err != nil {
		editAction := "/symbol-mappings/" + strconv.FormatInt(id, 10) + "/edit"
		cancelHref := "/symbol-mappings"

		// Re-parse broker symbols for form re-render
		brokerSymbols := parseBrokerSymbols(r)

		data := newSymbolMappingFormPageData(web.PageData{
			Title: "Edit Symbol Mapping",
			Error: symbolMappingUserFriendlyError(err),
		}, editAction, "Save Changes", cancelHref)
		data.InternalSymbol = internalSymbol
		data.MarketDataSymbol = marketDataSymbol
		data.BrokerSymbols = brokerSymbols

		if renderErr := h.renderer.Render(w, "symbol_mapping/form", data); renderErr != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		return
	}

	setFlash(w, "Symbol mapping updated successfully")
	http.Redirect(w, r, "/symbol-mappings", http.StatusSeeOther)
}

// HandleDeletePage handles POST /symbol-mappings/{id}/delete.
func (h *SymbolMappingWebHandler) HandleDeletePage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		if errors.Is(err, symbolmapping.ErrInUse) {
			http.Redirect(w, r, "/symbol-mappings?error=in_use", http.StatusSeeOther)
			return
		}
		http.NotFound(w, r)
		return
	}

	setFlash(w, "Symbol mapping deleted successfully")
	http.Redirect(w, r, "/symbol-mappings", http.StatusSeeOther)
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
