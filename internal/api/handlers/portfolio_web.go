package handlers

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/account"
	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/portfolio"
	"github.com/arch-portfolio-lab/portfoliolab/internal/web"
)

// Common currencies offered in the form dropdown.
var commonCurrencies = []string{
	"USD", "EUR", "GBP", "JPY", "CHF", "CAD", "AUD", "NZD",
	"SEK", "NOK", "DKK", "PLN", "CZK", "HUF", "TRY", "CNY",
	"INR", "KRW", "SGD", "HKD", "MXN", "BRL", "ZAR",
}

// portfolioFormPageData is the shared data struct for the portfolio form template.
// All handlers that render portfolio/form must use this struct to ensure all
// template fields are present and prevent "can't evaluate field" panics.
type portfolioFormPageData struct {
	web.PageData
	Name            string
	SelectedCurrency string
	Currencies      []string
	Action          string
	SubmitText      string
	CancelHref      string
}

// newPortfolioFormPageData creates a portfolioFormPageData with common defaults.
func newPortfolioFormPageData(pd web.PageData, action, submitText, cancelHref string) *portfolioFormPageData {
	return &portfolioFormPageData{
		PageData:         pd,
		Currencies:       commonCurrencies,
		Action:           action,
		SubmitText:       submitText,
		CancelHref:       cancelHref,
	}
}

// PortfolioWebHandler handles server-rendered portfolio pages.
type PortfolioWebHandler struct {
	service        *portfolio.Service
	accountService *account.Service
	renderer       *web.Renderer
}

// NewPortfolioWebHandler creates a new portfolio web handler.
func NewPortfolioWebHandler(service *portfolio.Service, accountService *account.Service, renderer *web.Renderer) *PortfolioWebHandler {
	return &PortfolioWebHandler{
		service:        service,
		accountService: accountService,
		renderer:       renderer,
	}
}

// RegisterRoutes mounts web portfolio routes on the given router.
// Note: more specific routes (with sub-paths) must be registered before catch-all routes.
func (h *PortfolioWebHandler) RegisterRoutes(r *chi.Mux) {
	// Specific routes first
	r.Post("/portfolios/{id}/delete", h.HandleDeletePage)
	r.Post("/portfolios/{id}/edit", h.HandleUpdatePage)
	r.Get("/portfolios/{id}/edit", h.HandleEditPage)
	r.Get("/portfolios/{id}", h.HandleDetailPage)
	r.Get("/portfolios/new", h.HandleNewPage)
	// Catch-all routes last
	r.Post("/portfolios", h.HandleCreatePage)
	r.Get("/portfolios", h.HandleListPage)
}

// HandleListPage renders GET /portfolios.
func (h *PortfolioWebHandler) HandleListPage(w http.ResponseWriter, r *http.Request) {
	portfolios, err := h.service.List(r.Context(), 0, 0)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	data := struct {
		web.PageData
		Portfolios []portfolio.Portfolio
	}{
		PageData: web.PageData{
			Title: "Portfolios",
			Flash: getFlash(r),
		},
		Portfolios: portfolios,
	}

	if data.Portfolios == nil {
		data.Portfolios = []portfolio.Portfolio{}
	}

	if err := h.renderer.Render(w, "portfolio/list", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleNewPage renders GET /portfolios/new.
func (h *PortfolioWebHandler) HandleNewPage(w http.ResponseWriter, r *http.Request) {
	data := newPortfolioFormPageData(web.PageData{
		Title: "New Portfolio",
	}, "/portfolios", "Create Portfolio", "/portfolios")

	if err := h.renderer.Render(w, "portfolio/form", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleCreatePage handles POST /portfolios (form submission).
func (h *PortfolioWebHandler) HandleCreatePage(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	currency := r.FormValue("currency")

	req := portfolio.CreateRequest{
		Name:     name,
		Currency: currency,
	}

	p, err := h.service.Create(r.Context(), req)
	if err != nil {
		data := newPortfolioFormPageData(web.PageData{
			Title: "New Portfolio",
			Error: userFriendlyError(err),
		}, "/portfolios", "Create Portfolio", "/portfolios")
		data.Name = name
		data.SelectedCurrency = currency

		if renderErr := h.renderer.Render(w, "portfolio/form", data); renderErr != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		return
	}

	setFlash(w, "Portfolio \""+p.Name+"\" created successfully")
	http.Redirect(w, r, "/portfolios", http.StatusSeeOther)
}

// portfolioDetail is the display struct for the portfolio detail page.
type portfolioDetail struct {
	ID        int64
	Name      string
	Currency  string
	CreatedAt string
	UpdatedAt string
}

// HandleDetailPage renders GET /portfolios/{id}.
func (h *PortfolioWebHandler) HandleDetailPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	p, err := h.service.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	accounts, err := h.accountService.ListByPortfolio(r.Context(), p.ID, 0, 0)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if accounts == nil {
		accounts = []account.Account{}
	}

	type accountRow struct {
		ID        int64
		Name      string
		CreatedAt string
	}
	rows := make([]accountRow, len(accounts))
	for i, a := range accounts {
		rows[i] = accountRow{
			ID:        a.ID,
			Name:      a.Name,
			CreatedAt: formatTime(a.CreatedAt),
		}
	}

	data := struct {
		web.PageData
		Portfolio portfolioDetail
		Accounts  []accountRow
	}{
		PageData: web.PageData{
			Title: p.Name,
			Flash: getFlash(r),
		},
		Portfolio: portfolioDetail{
			ID:        p.ID,
			Name:      p.Name,
			Currency:  p.Currency,
			CreatedAt: formatTime(p.CreatedAt),
			UpdatedAt: formatTime(p.UpdatedAt),
		},
		Accounts: rows,
	}

	if err := h.renderer.Render(w, "portfolio/detail", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleEditPage renders GET /portfolios/{id}/edit.
func (h *PortfolioWebHandler) HandleEditPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	p, err := h.service.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	editAction := "/portfolios/" + strconv.FormatInt(id, 10) + "/edit"
	cancelHref := "/portfolios/" + strconv.FormatInt(id, 10)
	data := newPortfolioFormPageData(web.PageData{
		Title: "Edit Portfolio",
	}, editAction, "Save Changes", cancelHref)
	data.Name = p.Name
	data.SelectedCurrency = p.Currency

	if err := h.renderer.Render(w, "portfolio/form", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleUpdatePage handles POST /portfolios/{id}/edit.
func (h *PortfolioWebHandler) HandleUpdatePage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	name := r.FormValue("name")
	currency := r.FormValue("currency")

	req := portfolio.UpdateRequest{}

	// Only include fields that were actually changed
	current, err := h.service.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if name != current.Name {
		req.Name = &name
	}
	if currency != current.Currency {
		req.Currency = &currency
	}

	p, err := h.service.Update(r.Context(), id, req)
	if err != nil {
		editAction := "/portfolios/" + strconv.FormatInt(id, 10) + "/edit"
		cancelHref := "/portfolios/" + strconv.FormatInt(id, 10)
		data := newPortfolioFormPageData(web.PageData{
			Title: "Edit Portfolio",
			Error: userFriendlyError(err),
		}, editAction, "Save Changes", cancelHref)
		data.Name = name
		data.SelectedCurrency = currency

		if renderErr := h.renderer.Render(w, "portfolio/form", data); renderErr != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		return
	}

	setFlash(w, "Portfolio updated successfully")
	http.Redirect(w, r, "/portfolios/"+strconv.FormatInt(p.ID, 10), http.StatusSeeOther)
}

// HandleDeletePage handles POST /portfolios/{id}/delete.
func (h *PortfolioWebHandler) HandleDeletePage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := h.service.Delete(r.Context(), id); err != nil {
		http.NotFound(w, r)
		return
	}

	setFlash(w, "Portfolio deleted successfully")
	http.Redirect(w, r, "/portfolios", http.StatusSeeOther)
}

// formatTime formats a time for display in templates.
func formatTime(t time.Time) string {
	return t.Format("2006-01-02 15:04")
}

// userFriendlyError returns a user-friendly message from a service error.
func userFriendlyError(err error) string {
	if errors.Is(err, portfolio.ErrInvalidName) {
		return "Invalid name: must be 1-100 characters"
	}
	if errors.Is(err, portfolio.ErrInvalidCurrency) {
		return "Invalid currency: must be a 3-letter ISO 4217 code (e.g. USD, EUR)"
	}
	if errors.Is(err, portfolio.ErrNameExists) {
		return "A portfolio with this name already exists"
	}
	return "An error occurred. Please try again."
}

// Flash cookie name.
const flashCookie = "__pl_flash"

// setFlash sets a one-time flash message cookie.
func setFlash(w http.ResponseWriter, message string) {
	cookie := &http.Cookie{
		Name:     flashCookie,
		Value:    url.QueryEscape(message),
		Path:     "/",
		MaxAge:   5, // Short-lived: 5 seconds
		HttpOnly: true,
	}
	http.SetCookie(w, cookie)
}

// getFlash reads and clears the flash message cookie. Returns the message.
// Callers should pass the ResponseWriter to clear the cookie.
func getFlash(r *http.Request) string {
	cookie, err := r.Cookie(flashCookie)
	if err != nil {
		return ""
	}

	message, _ := url.QueryUnescape(cookie.Value)
	return message
}

// clearFlash clears the flash cookie.
func clearFlash(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:   flashCookie,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
}
