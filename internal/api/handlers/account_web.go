package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/account"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/portfolio"
	"codeberg.org/eddiectc/portfoliolab/internal/web"
)

// accountFormPageData is the shared data struct for the account form template.
type accountFormPageData struct {
	web.PageData
	Name            string
	PortfolioID     int64
	Action          string
	SubmitText      string
	CancelHref      string
}

// newAccountFormPageData creates an accountFormPageData with common defaults.
func newAccountFormPageData(pd web.PageData, portfolioID int64, action, submitText, cancelHref string) *accountFormPageData {
	return &accountFormPageData{
		PageData:    pd,
		PortfolioID: portfolioID,
		Action:      action,
		SubmitText:  submitText,
		CancelHref:  cancelHref,
	}
}

// accountDetail is the display struct for the account detail page.
type accountDetail struct {
	ID              int64
	Name            string
	PortfolioID     int64
	PortfolioName   string
	CreatedAt       string
	UpdatedAt       string
}

// AccountWebHandler handles server-rendered account pages.
type AccountWebHandler struct {
	accountService *account.Service
	portfolioSvc   *portfolio.Service
	renderer       *web.Renderer
}

// NewAccountWebHandler creates a new account web handler.
func NewAccountWebHandler(accountService *account.Service, portfolioSvc *portfolio.Service, renderer *web.Renderer) *AccountWebHandler {
	return &AccountWebHandler{
		accountService: accountService,
		portfolioSvc:   portfolioSvc,
		renderer:       renderer,
	}
}

// RegisterRoutes mounts web account routes on the given router.
// Note: more specific routes (with sub-paths) must be registered before catch-all routes.
func (h *AccountWebHandler) RegisterRoutes(r *chi.Mux) {
	// Specific routes first
	r.Post("/accounts/{id}/delete", h.HandleDeletePage)
	r.Post("/accounts/{id}/edit", h.HandleUpdatePage)
	r.Get("/accounts/{id}/edit", h.HandleEditPage)
	r.Get("/accounts/{id}", h.HandleDetailPage)
	r.Get("/accounts/new", h.HandleNewPage)
	// Catch-all route last
	r.Post("/accounts", h.HandleCreatePage)
}

// HandleNewPage renders GET /accounts/new?portfolio_id=X.
// Requires portfolio_id query parameter; redirects to /portfolios if missing.
func (h *AccountWebHandler) HandleNewPage(w http.ResponseWriter, r *http.Request) {
	portfolioIDStr := r.URL.Query().Get("portfolio_id")
	if portfolioIDStr == "" {
		http.Redirect(w, r, "/portfolios", http.StatusSeeOther)
		return
	}

	portfolioID, err := strconv.ParseInt(portfolioIDStr, 10, 64)
	if err != nil || portfolioID == 0 {
		http.Redirect(w, r, "/portfolios", http.StatusSeeOther)
		return
	}

	cancelHref := "/portfolios/" + strconv.FormatInt(portfolioID, 10)

	data := newAccountFormPageData(web.PageData{
		Title: "New Account",
	}, portfolioID, "/accounts", "Create Account", cancelHref)

	if err := h.renderer.Render(w, "account/form", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleCreatePage handles POST /accounts (form submission).
func (h *AccountWebHandler) HandleCreatePage(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	portfolioID, _ := strconv.ParseInt(r.FormValue("portfolio_id"), 10, 64)

	req := account.CreateRequest{
		Name:        name,
		PortfolioID: portfolioID,
	}

	cancelHref := "/portfolios/" + strconv.FormatInt(portfolioID, 10)

	a, err := h.accountService.Create(r.Context(), req)
	if err != nil {
		data := newAccountFormPageData(web.PageData{
			Title: "New Account",
			Error: accountUserFriendlyError(err),
		}, portfolioID, "/accounts", "Create Account", cancelHref)
		data.Name = name

		if renderErr := h.renderer.Render(w, "account/form", data); renderErr != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		return
	}

	setFlash(w, "Account \""+a.Name+"\" created successfully")
	http.Redirect(w, r, "/portfolios/"+strconv.FormatInt(a.PortfolioID, 10), http.StatusSeeOther)
}

// HandleDetailPage renders GET /accounts/{id}.
func (h *AccountWebHandler) HandleDetailPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	a, err := h.accountService.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Get portfolio name
	portfolios, _ := h.portfolioSvc.List(r.Context(), 0, 0)
	portfolioName := ""
	for _, p := range portfolios {
		if p.ID == a.PortfolioID {
			portfolioName = p.Name
			break
		}
	}

	data := struct {
		web.PageData
		Account accountDetail
	}{
		PageData: web.PageData{
			Title: a.Name,
		},
		Account: accountDetail{
			ID:            a.ID,
			Name:          a.Name,
			PortfolioID:   a.PortfolioID,
			PortfolioName: portfolioName,
			CreatedAt:     formatTime(a.CreatedAt),
			UpdatedAt:     formatTime(a.UpdatedAt),
		},
	}

	if err := h.renderer.Render(w, "account/detail", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleEditPage renders GET /accounts/{id}/edit.
func (h *AccountWebHandler) HandleEditPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	a, err := h.accountService.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	editAction := "/accounts/" + strconv.FormatInt(id, 10) + "/edit"
	cancelHref := "/accounts/" + strconv.FormatInt(id, 10)
	data := newAccountFormPageData(web.PageData{
		Title: "Edit Account",
	}, a.PortfolioID, editAction, "Save Changes", cancelHref)
	data.Name = a.Name

	if err := h.renderer.Render(w, "account/form", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleUpdatePage handles POST /accounts/{id}/edit.
func (h *AccountWebHandler) HandleUpdatePage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	name := r.FormValue("name")
	portfolioID, _ := strconv.ParseInt(r.FormValue("portfolio_id"), 10, 64)

	req := account.UpdateRequest{}

	// Only include fields that were actually changed
	current, err := h.accountService.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if name != current.Name {
		req.Name = &name
	}
	if portfolioID != current.PortfolioID {
		req.PortfolioID = &portfolioID
	}

	a, err := h.accountService.Update(r.Context(), id, req)
	if err != nil {
		editAction := "/accounts/" + strconv.FormatInt(id, 10) + "/edit"
		cancelHref := "/accounts/" + strconv.FormatInt(id, 10)
		data := newAccountFormPageData(web.PageData{
			Title: "Edit Account",
			Error: accountUserFriendlyError(err),
		}, portfolioID, editAction, "Save Changes", cancelHref)
		data.Name = name

		if renderErr := h.renderer.Render(w, "account/form", data); renderErr != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		return
	}

	setFlash(w, "Account updated successfully")
	http.Redirect(w, r, "/accounts/"+strconv.FormatInt(a.ID, 10), http.StatusSeeOther)
}

// HandleDeletePage handles POST /accounts/{id}/delete.
// Redirects back to the account's portfolio page.
func (h *AccountWebHandler) HandleDeletePage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	a, err := h.accountService.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := h.accountService.Delete(r.Context(), id); err != nil {
		http.NotFound(w, r)
		return
	}

	setFlash(w, "Account deleted successfully")
	http.Redirect(w, r, "/portfolios/"+strconv.FormatInt(a.PortfolioID, 10), http.StatusSeeOther)
}

// accountUserFriendlyError returns a user-friendly message from an account service error.
func accountUserFriendlyError(err error) string {
	if errors.Is(err, account.ErrInvalidName) {
		return "Invalid name: must be 1-100 characters"
	}
	if errors.Is(err, account.ErrNameExists) {
		return "An account with this name already exists"
	}
	if errors.Is(err, account.ErrPortfolioNotFound) {
		return "Selected portfolio not found"
	}
	return "An error occurred. Please try again."
}
