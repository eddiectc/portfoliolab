package handlers

import (
	"encoding/base64"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/account"
	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/ibkrimport"
	"github.com/arch-portfolio-lab/portfoliolab/internal/web"
)

// ImportWebHandler handles server-rendered IBKR Flex XML import pages.
type ImportWebHandler struct {
	importSvc   ImportService
	accountSvc  *account.Service
	renderer    *web.Renderer
	maxFileSize int64
}

// NewImportWebHandler creates a new IBKR import web handler.
func NewImportWebHandler(importSvc ImportService, accountSvc *account.Service, renderer *web.Renderer) *ImportWebHandler {
	return &ImportWebHandler{
		importSvc:   importSvc,
		accountSvc:  accountSvc,
		renderer:    renderer,
		maxFileSize: 50 << 20, // 50 MB
	}
}

// RegisterRoutes mounts web import routes on the given router.
// More specific routes are registered before catch-all routes.
func (h *ImportWebHandler) RegisterRoutes(r *chi.Mux) {
	r.Post("/transactions/import/ibkr/confirm", h.HandleConfirmPost)
	r.Post("/transactions/import/ibkr", h.HandleImportPost)
	r.Get("/transactions/import/ibkr", h.HandleImportPage)
}

// importPageData holds data for the upload page.
type importPageData struct {
	web.PageData
	Accounts []account.Account
}

// previewPageData holds data for the preview page.
type previewPageData struct {
	web.PageData
	Preview   *ibkrimport.PreviewResponse
	XMLData   string // base64-encoded XML for re-submission
	AccountID string // pre-selected account
}

// HandleImportPage renders GET /transactions/import/ibkr — the upload page.
func (h *ImportWebHandler) HandleImportPage(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.accountSvc.List(r.Context(), 0, 0)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if accounts == nil {
		accounts = []account.Account{}
	}

	data := importPageData{
		PageData: web.PageData{
			Title: "Import IBKR Flex XML",
			Flash: getFlash(w, r),
		},
		Accounts: accounts,
	}

	if err := h.renderer.Render(w, "transaction/import", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleImportPost handles POST /transactions/import/ibkr — upload XML and show preview.
func (h *ImportWebHandler) HandleImportPost(w http.ResponseWriter, r *http.Request) {
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

	xmlData, err := readXMLFile(r)
	if err != nil {
		h.renderImportPage(w, r, "Failed to read XML file: "+err.Error())
		return
	}

	preview, err := h.importSvc.Preview(r.Context(), xmlData, accountID)
	if err != nil {
		h.renderImportPage(w, r, importUserFriendlyError(err))
		return
	}

	// Encode XML data for re-submission on confirm
	encodedXML := base64.StdEncoding.EncodeToString(xmlData)

	data := previewPageData{
		PageData: web.PageData{
			Title: "Import Preview",
		},
		Preview:   preview,
		XMLData:   encodedXML,
		AccountID: accountIDStr,
	}

	if err := h.renderer.Render(w, "transaction/import_preview", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleConfirmPost handles POST /transactions/import/ibkr/confirm — confirm the import.
func (h *ImportWebHandler) HandleConfirmPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(h.maxFileSize); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	accountID, err := parseAccountID(r.FormValue("account_id"))
	if err != nil {
		http.Error(w, "invalid account_id", http.StatusBadRequest)
		return
	}

	// Decode base64-encoded XML data
	encodedXML := r.FormValue("xml_data")
	if encodedXML == "" {
		http.Error(w, "missing xml_data", http.StatusBadRequest)
		return
	}

	xmlData, err := base64.StdEncoding.DecodeString(encodedXML)
	if err != nil {
		http.Error(w, "invalid xml_data encoding", http.StatusBadRequest)
		return
	}

	result, err := h.importSvc.ConfirmImport(r.Context(), xmlData, accountID)
	if err != nil {
		// Re-render preview page with error
		preview, previewErr := h.importSvc.Preview(r.Context(), xmlData, accountID)
		if previewErr != nil || preview == nil {
			setFlash(w, "Import failed. Please try again.")
			http.Redirect(w, r, "/transactions/import/ibkr", http.StatusSeeOther)
			return
		}

		data := previewPageData{
			PageData: web.PageData{
				Title: "Import Preview",
				Error: "Import failed: " + err.Error(),
			},
			Preview:   preview,
			XMLData:   encodedXML,
			AccountID: strconv.FormatInt(accountID, 10),
		}

		if renderErr := h.renderer.Render(w, "transaction/import_preview", data); renderErr != nil {
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
func (h *ImportWebHandler) renderImportPage(w http.ResponseWriter, r *http.Request, errMsg string) {
	accounts, _ := h.accountSvc.List(r.Context(), 0, 0)
	if accounts == nil {
		accounts = []account.Account{}
	}

	data := importPageData{
		PageData: web.PageData{
			Title: "Import IBKR Flex XML",
			Error: errMsg,
		},
		Accounts: accounts,
	}

	if err := h.renderer.Render(w, "transaction/import", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// importUserFriendlyError returns a user-friendly message from an import error.
func importUserFriendlyError(err error) string {
	if err == ibkrimport.ErrAccountNotFound {
		return "Selected account not found"
	}
	if err == ibkrimport.ErrInvalidXML {
		return "Invalid XML: " + err.Error()
	}
	return "Failed to parse XML: " + err.Error()
}
