package handlers

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/modelportfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/symbolmapping"
	"codeberg.org/eddiectc/portfoliolab/internal/web"
)

// modelPortfolioFormPageData is the shared data struct for the model portfolio form template.
type modelPortfolioFormPageData struct {
	web.PageData
	Name       string
	Entries    []modelPortfolioEntryForm
	Symbols    []symbolmapping.SymbolMapping
	Action     string
	SubmitText string
	CancelHref string
}

// modelPortfolioEntryForm holds a single symbol+weight row for the form.
type modelPortfolioEntryForm struct {
	Symbol    string
	WeightPct string
}

// newModelPortfolioFormPageData creates a modelPortfolioFormPageData with common defaults.
func newModelPortfolioFormPageData(pd web.PageData, symbols []symbolmapping.SymbolMapping, entries []modelPortfolioEntryForm, action, submitText, cancelHref string) *modelPortfolioFormPageData {
	if entries == nil {
		entries = []modelPortfolioEntryForm{}
	}
	return &modelPortfolioFormPageData{
		PageData:   pd,
		Entries:    entries,
		Symbols:    symbols,
		Action:     action,
		SubmitText: submitText,
		CancelHref: cancelHref,
	}
}

// ModelPortfolioWebHandler handles server-rendered model portfolio pages.
type ModelPortfolioWebHandler struct {
	service   *modelportfolio.Service
	symbolSvc *symbolmapping.Service
	renderer  *web.Renderer
}

// NewModelPortfolioWebHandler creates a new model portfolio web handler.
func NewModelPortfolioWebHandler(service *modelportfolio.Service, symbolSvc *symbolmapping.Service, renderer *web.Renderer) *ModelPortfolioWebHandler {
	return &ModelPortfolioWebHandler{
		service:   service,
		symbolSvc: symbolSvc,
		renderer:  renderer,
	}
}

// RegisterRoutes mounts web model portfolio routes on the given router.
// Note: more specific routes (with sub-paths) must be registered before catch-all routes.
func (h *ModelPortfolioWebHandler) RegisterRoutes(r *chi.Mux) {
	// Specific routes first
	r.Post("/model-portfolios/{id}/delete", h.HandleDeletePage)
	r.Post("/model-portfolios/{id}/edit", h.HandleEditPost)
	r.Get("/model-portfolios/{id}/edit", h.HandleEditPage)
	r.Get("/model-portfolios/new", h.HandleNewPage)
	// Catch-all routes last
	r.Post("/model-portfolios", h.HandleCreatePage)
	r.Get("/model-portfolios", h.HandleListPage)
}

// HandleListPage renders GET /model-portfolios.
func (h *ModelPortfolioWebHandler) HandleListPage(w http.ResponseWriter, r *http.Request) {
	modelPortfolios, err := h.service.ListAll(r.Context())
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	data := struct {
		web.PageData
		ModelPortfolios []modelportfolio.ModelPortfolio
	}{
		PageData: web.PageData{
			Title: "Model Portfolios",
			Flash: getFlash(w, r),
		},
		ModelPortfolios: modelPortfolios,
	}

	if data.ModelPortfolios == nil {
		data.ModelPortfolios = []modelportfolio.ModelPortfolio{}
	}

	if err := h.renderer.Render(w, "model_portfolio/list", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleNewPage renders GET /model-portfolios/new.
func (h *ModelPortfolioWebHandler) HandleNewPage(w http.ResponseWriter, r *http.Request) {
	symbols, err := h.symbolSvc.ListAll(r.Context())
	if err != nil {
		slog.Warn("failed to fetch symbols for autocomplete", "error", err)
	}
	if symbols == nil {
		symbols = []symbolmapping.SymbolMapping{}
	}

	data := newModelPortfolioFormPageData(web.PageData{
		Title: "New Model Portfolio",
	}, symbols, []modelPortfolioEntryForm{}, "/model-portfolios", "Create Model Portfolio", "/model-portfolios")

	if err := h.renderer.Render(w, "model_portfolio/form", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// parseEntries extracts symbol+weight pairs from form values.
// Form fields are named "symbol_0", "weight_0", "symbol_1", "weight_1", etc.
func parseEntries(r *http.Request) []modelportfolio.ModelPortfolioEntry {
	symbols := r.Form["symbol"]
	weights := r.Form["weight"]

	count := len(symbols)
	if len(weights) < count {
		count = len(weights)
	}

	var result []modelportfolio.ModelPortfolioEntry
	for i := 0; i < count; i++ {
		symbol := strings.TrimSpace(symbols[i])
		weightStr := strings.TrimSpace(weights[i])
		if symbol != "" && weightStr != "" {
			weight, err := decimal.Parse(weightStr)
			if err != nil {
				weight = decimal.Zero
			}
			result = append(result, modelportfolio.ModelPortfolioEntry{
				Symbol:    symbol,
				WeightPct: weight,
			})
		}
	}
	return result
}

// parseEntriesForm extracts symbol+weight pairs as form-display values.
func parseEntriesForm(r *http.Request) []modelPortfolioEntryForm {
	symbols := r.Form["symbol"]
	weights := r.Form["weight"]

	count := len(symbols)
	if len(weights) < count {
		count = len(weights)
	}

	var result []modelPortfolioEntryForm
	for i := 0; i < count; i++ {
		symbol := strings.TrimSpace(symbols[i])
		weightStr := strings.TrimSpace(weights[i])
		if symbol != "" || weightStr != "" {
			result = append(result, modelPortfolioEntryForm{
				Symbol:    symbol,
				WeightPct: weightStr,
			})
		}
	}
	return result
}

// HandleCreatePage handles POST /model-portfolios (form submission).
func (h *ModelPortfolioWebHandler) HandleCreatePage(w http.ResponseWriter, r *http.Request) {
	symbols, err := h.symbolSvc.ListAll(r.Context())
	if err != nil {
		slog.Warn("failed to fetch symbols for autocomplete", "error", err)
	}
	if symbols == nil {
		symbols = []symbolmapping.SymbolMapping{}
	}

	name := strings.TrimSpace(r.FormValue("name"))
	entries := parseEntries(r)

	req := modelportfolio.CreateRequest{
		Name:    name,
		Entries: entries,
	}

	mp, err := h.service.Create(r.Context(), req)
	if err != nil {
		formEntries := parseEntriesForm(r)
		data := newModelPortfolioFormPageData(web.PageData{
			Title: "New Model Portfolio",
			Error: modelPortfolioUserFriendlyError(err),
		}, symbols, formEntries, "/model-portfolios", "Create Model Portfolio", "/model-portfolios")
		data.Name = name

		if renderErr := h.renderer.Render(w, "model_portfolio/form", data); renderErr != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		return
	}

	setFlash(w, "Model portfolio \""+mp.Name+"\" created successfully")
	http.Redirect(w, r, "/model-portfolios", http.StatusSeeOther)
}

// HandleEditPage renders GET /model-portfolios/{id}/edit.
func (h *ModelPortfolioWebHandler) HandleEditPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	mp, err := h.service.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	symbols, err := h.symbolSvc.ListAll(r.Context())
	if err != nil {
		slog.Warn("failed to fetch symbols for autocomplete", "error", err)
	}
	if symbols == nil {
		symbols = []symbolmapping.SymbolMapping{}
	}

	var formEntries []modelPortfolioEntryForm
	for _, e := range mp.Entries {
		formEntries = append(formEntries, modelPortfolioEntryForm{
			Symbol:    e.Symbol,
			WeightPct: e.WeightPct.String(),
		})
	}

	editAction := "/model-portfolios/" + strconv.FormatInt(id, 10) + "/edit"
	cancelHref := "/model-portfolios"

	data := newModelPortfolioFormPageData(web.PageData{
		Title: "Edit Model Portfolio",
	}, symbols, formEntries, editAction, "Save Changes", cancelHref)
	data.Name = mp.Name

	if err := h.renderer.Render(w, "model_portfolio/form", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleEditPost handles POST /model-portfolios/{id}/edit.
func (h *ModelPortfolioWebHandler) HandleEditPost(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	symbols, err := h.symbolSvc.ListAll(r.Context())
	if err != nil {
		slog.Warn("failed to fetch symbols for autocomplete", "error", err)
	}
	if symbols == nil {
		symbols = []symbolmapping.SymbolMapping{}
	}

	name := strings.TrimSpace(r.FormValue("name"))
	entries := parseEntries(r)

	req := modelportfolio.UpdateRequest{
		Name:    &name,
		Entries: entries,
	}

	_, err = h.service.Update(r.Context(), id, req)
	if err != nil {
		formEntries := parseEntriesForm(r)
		editAction := "/model-portfolios/" + strconv.FormatInt(id, 10) + "/edit"
		cancelHref := "/model-portfolios"

		data := newModelPortfolioFormPageData(web.PageData{
			Title: "Edit Model Portfolio",
			Error: modelPortfolioUserFriendlyError(err),
		}, symbols, formEntries, editAction, "Save Changes", cancelHref)
		data.Name = name

		if renderErr := h.renderer.Render(w, "model_portfolio/form", data); renderErr != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		return
	}

	setFlash(w, "Model portfolio updated successfully")
	http.Redirect(w, r, "/model-portfolios", http.StatusSeeOther)
}

// HandleDeletePage handles POST /model-portfolios/{id}/delete.
func (h *ModelPortfolioWebHandler) HandleDeletePage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		http.NotFound(w, r)
		return
	}

	setFlash(w, "Model portfolio deleted successfully")
	http.Redirect(w, r, "/model-portfolios", http.StatusSeeOther)
}

// modelPortfolioUserFriendlyError returns a user-friendly message from a model portfolio service error.
func modelPortfolioUserFriendlyError(err error) string {
	if errors.Is(err, modelportfolio.ErrInvalidName) {
		return "Invalid name: must be 1-100 characters"
	}
	if errors.Is(err, modelportfolio.ErrNameExists) {
		return "A model portfolio with this name already exists"
	}
	if errors.Is(err, modelportfolio.ErrEmptyEntries) {
		return "At least one entry is required"
	}
	if errors.Is(err, modelportfolio.ErrInvalidWeight) {
		return "Each weight must be greater than 0%"
	}
	if errors.Is(err, modelportfolio.ErrDuplicateSymbol) {
		return "Entries contain duplicate symbols"
	}
	if mpErr := new(modelportfolio.ModelPortfolioError); errors.As(err, &mpErr) {
		return mpErr.Message
	}
	return "An error occurred. Please try again."
}
