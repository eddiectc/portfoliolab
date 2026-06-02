package handlers

import (
	"context"
	"encoding/base64"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/account"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/brokerimport"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/symbolmapping"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/trading212import"
	"codeberg.org/eddiectc/portfoliolab/internal/web"
)

// Trading212ImportWebHandler handles server-rendered Trading 212 CSV import pages.
type Trading212ImportWebHandler struct {
	importSvc        ImportService
	accountSvc       *account.Service
	symbolMappingSvc *symbolmapping.Service
	renderer         *web.Renderer
	maxFileSize      int64
}

// NewTrading212ImportWebHandler creates a new Trading 212 import web handler.
func NewTrading212ImportWebHandler(importSvc ImportService, accountSvc *account.Service, symbolMappingSvc *symbolmapping.Service, renderer *web.Renderer) *Trading212ImportWebHandler {
	return &Trading212ImportWebHandler{
		importSvc:        importSvc,
		accountSvc:       accountSvc,
		symbolMappingSvc: symbolMappingSvc,
		renderer:         renderer,
		maxFileSize:      50 << 20, // 50 MB
	}
}

// RegisterRoutes mounts web import routes on the given router.
// More specific routes are registered before catch-all routes.
func (h *Trading212ImportWebHandler) RegisterRoutes(r *chi.Mux) {
	r.Post("/transactions/import/trading212/confirm", h.HandleConfirmPost)
	r.Post("/transactions/import/trading212", h.HandleImportPost)
	r.Get("/transactions/import/trading212", h.HandleImportPage)
}

// t212ImportPageData holds data for the upload page.
type t212ImportPageData struct {
	web.PageData
	Accounts []account.Account
}

// t212PreviewPageData holds data for the preview page.
type t212PreviewPageData struct {
	web.PageData
	Preview         *brokerimport.PreviewResponse
	CSVData         string   // base64-encoded CSV for re-submission
	AccountID       string   // pre-selected account
	ExistingSymbols []string // internal symbols already in the system
	ActiveTab       string   // tab to activate on load (e.g. "skipped")
}

// HandleImportPage renders GET /transactions/import/trading212 — the upload page.
func (h *Trading212ImportWebHandler) HandleImportPage(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.accountSvc.ListAll(r.Context())
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if accounts == nil {
		accounts = []account.Account{}
	}

	data := t212ImportPageData{
		PageData: web.PageData{
			Title: "Import Trading 212 CSV",
			Flash: getFlash(w, r),
		},
		Accounts: accounts,
	}

	if err := h.renderer.Render(w, "transaction/t212_import", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleImportPost handles POST /transactions/import/trading212 — upload CSV and show preview.
func (h *Trading212ImportWebHandler) HandleImportPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(h.maxFileSize); err != nil {
		h.renderImportPage(w, r, "File too large or invalid form data")
		return
	}

	accountIDStr := r.FormValue("account_id")
	if accountIDStr == "" {
		h.renderImportPage(w, r, "Please select an account")
		return
	}

	accountID, err := strconv.ParseInt(accountIDStr, 10, 64)
	if err != nil || accountID == 0 {
		h.renderImportPage(w, r, "Invalid account selection")
		return
	}

	// Accept either a file upload or pre-encoded CSV data (for re-submission after resolving symbols)
	var csvData []byte
	encodedCSV := r.FormValue("csv_data")
	if encodedCSV != "" {
		csvData, err = base64.StdEncoding.DecodeString(encodedCSV)
		if err != nil {
			h.renderImportPage(w, r, "Invalid CSV data")
			return
		}
	} else {
		csvData, err = readCSVFile(r)
		if err != nil {
			h.renderImportPage(w, r, "Failed to read CSV file: "+err.Error())
			return
		}
		encodedCSV = base64.StdEncoding.EncodeToString(csvData)
	}

	preview, err := h.importSvc.Preview(r.Context(), csvData, accountID)
	if err != nil {
		h.renderImportPage(w, r, t212importUserFriendlyError(err))
		return
	}

	// Fetch existing internal symbols for client-side existence check in the resolve modal
	existingSymbols := h.getExistingSymbols(r.Context())

	// Read tab query param (e.g. "skipped" after resolving a symbol)
	activeTab := r.URL.Query().Get("tab")
	if activeTab == "" {
		activeTab = "all"
	}

	data := t212PreviewPageData{
		PageData:        web.PageData{Title: "Import Preview"},
		Preview:         preview,
		CSVData:         encodedCSV,
		AccountID:       accountIDStr,
		ExistingSymbols: existingSymbols,
		ActiveTab:       activeTab,
	}

	if err := h.renderer.Render(w, "transaction/t212_import_preview", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleConfirmPost handles POST /transactions/import/trading212/confirm — confirm the import.
func (h *Trading212ImportWebHandler) HandleConfirmPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(h.maxFileSize); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	accountID, err := parseAccountID(r.FormValue("account_id"))
	if err != nil {
		http.Error(w, "invalid account_id", http.StatusBadRequest)
		return
	}

	// Decode base64-encoded CSV data
	encodedCSV := r.FormValue("csv_data")
	if encodedCSV == "" {
		http.Error(w, "missing csv_data", http.StatusBadRequest)
		return
	}

	csvData, err := base64.StdEncoding.DecodeString(encodedCSV)
	if err != nil {
		http.Error(w, "invalid csv_data encoding", http.StatusBadRequest)
		return
	}

	result, err := h.importSvc.ConfirmImport(r.Context(), csvData, accountID)
	if err != nil {
		// Re-render preview page with error
		preview, previewErr := h.importSvc.Preview(r.Context(), csvData, accountID)
		if previewErr != nil || preview == nil {
			setFlash(w, "Import failed. Please try again.")
			http.Redirect(w, r, "/transactions/import/trading212", http.StatusSeeOther)
			return
		}

		data := t212PreviewPageData{
			PageData: web.PageData{
				Title: "Import Preview",
				Error: "Import failed: " + err.Error(),
			},
			Preview:         preview,
			CSVData:         encodedCSV,
			AccountID:       strconv.FormatInt(accountID, 10),
			ExistingSymbols: h.getExistingSymbols(r.Context()),
			ActiveTab:       "all",
		}

		if renderErr := h.renderer.Render(w, "transaction/t212_import_preview", data); renderErr != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
		return
	}

	// Redirect to transactions list with flash summary
	message := "Imported " + strconv.Itoa(result.CreatedCount) + " transaction(s)"
	if result.SkippedCount > 0 {
		message += ", skipped " + strconv.Itoa(result.SkippedCount)
	}
	setFlash(w, message)
	http.Redirect(w, r, "/transactions", http.StatusSeeOther)
}

// renderImportPage re-renders the upload page with an error message.
func (h *Trading212ImportWebHandler) renderImportPage(w http.ResponseWriter, r *http.Request, errMsg string) {
	accounts, _ := h.accountSvc.ListAll(r.Context())
	if accounts == nil {
		accounts = []account.Account{}
	}

	data := t212ImportPageData{
		PageData: web.PageData{
			Title: "Import Trading 212 CSV",
			Error: errMsg,
		},
		Accounts: accounts,
	}

	if err := h.renderer.Render(w, "transaction/t212_import", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// t212importUserFriendlyError returns a user-friendly message from an import error.
func t212importUserFriendlyError(err error) string {
	if err == trading212import.ErrAccountNotFound {
		return "Selected account not found"
	}
	if err == trading212import.ErrInvalidCSV {
		return "Invalid CSV: " + err.Error()
	}
	return "Failed to parse CSV: " + err.Error()
}

// getExistingSymbols fetches all unique internal symbols from the system
// for client-side existence checks in the resolve modal.
func (h *Trading212ImportWebHandler) getExistingSymbols(ctx context.Context) []string {
	mappings, err := h.symbolMappingSvc.List(ctx, 0, 0)
	if err != nil {
		return nil
	}

	seen := make(map[string]struct{})
	var symbols []string
	for _, m := range mappings {
		if _, ok := seen[m.InternalSymbol]; !ok {
			seen[m.InternalSymbol] = struct{}{}
			symbols = append(symbols, m.InternalSymbol)
		}
	}
	return symbols
}
