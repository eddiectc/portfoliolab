package handlers

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/govalues/decimal"

	"codeberg.org/eddiectc/portfoliolab/internal/domain/account"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/symbolmapping"
	"codeberg.org/eddiectc/portfoliolab/internal/domain/transaction"
	"codeberg.org/eddiectc/portfoliolab/internal/web"
)

const defaultPageLimit = 20

// allowedTransactionTypes is the list of valid transaction types for dropdowns.
var allowedTransactionTypes = []string{
	"buy", "sell", "deposit", "withdrawal", "dividend", "interest", "fee", "tax",
}

// transactionFormPageData is the shared data struct for the transaction form template.
type transactionFormPageData struct {
	web.PageData
	Transaction   *transaction.Transaction
	Accounts      []account.Account
	Symbols       []symbolmapping.SymbolMapping
	Types         []string
	FieldErrors   map[string]string
	// Form values (preserved on error)
	AccountID      string
	Date           string
	Type           string
	Symbol         string
	Quantity       string
	Price          string
	Currency       string
	NetCash        string
	Description    string
	LotID          string
	ExternalSystem string
	ExternalRef    string
	// Action
	Action     string
	SubmitText string
	CancelHref string
}

// newTransactionFormPageData creates a transactionFormPageData with common defaults.
func newTransactionFormPageData(pd web.PageData, accounts []account.Account, symbols []symbolmapping.SymbolMapping, action, submitText, cancelHref string) *transactionFormPageData {
	return &transactionFormPageData{
		PageData:   pd,
		Accounts:   accounts,
		Symbols:    symbols,
		Types:      allowedTransactionTypes,
		FieldErrors: make(map[string]string),
		Action:     action,
		SubmitText: submitText,
		CancelHref: cancelHref,
	}
}

// transactionListPageData is the data struct for the transaction list template.
type transactionListPageData struct {
	web.PageData
	Transactions []transaction.TransactionWithAccount
	Accounts     []account.Account
	Types        []string
	Filter       TransactionFilter
	Page         int
	HasPrev      bool
	HasNext      bool
}

// TransactionFilter holds parsed filter parameters from query string.
type TransactionFilter struct {
	AccountID string
	Symbol    string
	Type      string
	DateFrom  string
	DateTo    string
}

// QueryParams serializes non-empty filter fields into a URL query fragment
// like "&account_id=1&symbol=AAPL". Returns "" if all fields are empty.
// Implements web.FilterEncoder for type-safe query preservation in templates.
// Deprecated: use PaginationQuery instead, which properly URL-encodes values.
func (f TransactionFilter) QueryParams() string {
	var parts []string
	if f.AccountID != "" {
		parts = append(parts, "account_id="+f.AccountID)
	}
	if f.Symbol != "" {
		parts = append(parts, "symbol="+f.Symbol)
	}
	if f.Type != "" {
		parts = append(parts, "type="+f.Type)
	}
	if f.DateFrom != "" {
		parts = append(parts, "date_from="+f.DateFrom)
	}
	if f.DateTo != "" {
		parts = append(parts, "date_to="+f.DateTo)
	}
	if len(parts) == 0 {
		return ""
	}
	return "&" + strings.Join(parts, "&")
}

// PaginationQuery returns a complete query string with the given page number
// and all non-empty filter fields, properly URL-encoded for use in href attributes.
// e.g., "?page=2&type=buy&symbol=AAPL"
func (f TransactionFilter) PaginationQuery(page int) string {
	values := url.Values{}
	values.Set("page", strconv.Itoa(page))
	if f.AccountID != "" {
		values.Set("account_id", f.AccountID)
	}
	if f.Symbol != "" {
		values.Set("symbol", f.Symbol)
	}
	if f.Type != "" {
		values.Set("type", f.Type)
	}
	if f.DateFrom != "" {
		values.Set("date_from", f.DateFrom)
	}
	if f.DateTo != "" {
		values.Set("date_to", f.DateTo)
	}
	return "?" + values.Encode()
}

// TransactionWebHandler handles server-rendered transaction pages.
type TransactionWebHandler struct {
	transactionSvc *transaction.Service
	accountSvc     *account.Service
	symbolSvc      *symbolmapping.Service
	renderer       *web.Renderer
}

// NewTransactionWebHandler creates a new transaction web handler.
func NewTransactionWebHandler(transactionSvc *transaction.Service, accountSvc *account.Service, symbolSvc *symbolmapping.Service, renderer *web.Renderer) *TransactionWebHandler {
	return &TransactionWebHandler{
		transactionSvc: transactionSvc,
		accountSvc:     accountSvc,
		symbolSvc:      symbolSvc,
		renderer:       renderer,
	}
}

// RegisterRoutes mounts web transaction routes on the given router.
// Note: more specific routes (with sub-paths) must be registered before catch-all routes.
func (h *TransactionWebHandler) RegisterRoutes(r *chi.Mux) {
	// Specific routes first
	r.Post("/transactions/{id}/delete", h.HandleDeletePage)
	r.Post("/transactions/{id}/edit", h.HandleEditPost)
	r.Get("/transactions/{id}/edit", h.HandleEditPage)
	r.Get("/transactions/{id}", h.HandleDetailPage)
	r.Get("/transactions/new", h.HandleNewPage)
	// Catch-all routes last
	r.Post("/transactions", h.HandleCreatePage)
	r.Get("/transactions", h.HandleListPage)
}

// HandleListPage renders GET /transactions with optional filters and pagination.
func (h *TransactionWebHandler) HandleListPage(w http.ResponseWriter, r *http.Request) {
	filter := parseTransactionFilter(r.URL.Query())
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := defaultPageLimit
	offset := (page - 1) * limit

	// Build domain filters
	var filters transaction.ListFilters
	if filter.AccountID != "" {
		if n, err := strconv.ParseInt(filter.AccountID, 10, 64); err == nil {
			filters.AccountID = &n
		}
	}
	if filter.Symbol != "" {
		s := strings.TrimSpace(filter.Symbol)
		filters.Symbol = &s
	}
	if filter.Type != "" {
		t := strings.TrimSpace(filter.Type)
		filters.Type = &t
	}
	if filter.DateFrom != "" {
		if t, err := time.Parse("2006-01-02", filter.DateFrom); err == nil {
			filters.DateFrom = &t
		}
	}
	if filter.DateTo != "" {
		if t, err := time.Parse("2006-01-02", filter.DateTo); err == nil {
			filters.DateTo = &t
		}
	}

	withAccount, err := h.transactionSvc.ListWithAccount(r.Context(), filters, limit, offset)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Fetch accounts for filter dropdown.
	accounts, _ := h.accountSvc.List(r.Context(), 0, 0)

	data := transactionListPageData{
		PageData: web.PageData{
			Title: "Transactions",
			Flash: getFlash(w, r),
		},
		Transactions: withAccount,
		Accounts:     accounts,
		Types:        allowedTransactionTypes,
		Filter:       filter,
		Page:         page,
		HasPrev:      page > 1,
		HasNext:      len(withAccount) == limit,
	}

	if data.Transactions == nil {
		data.Transactions = []transaction.TransactionWithAccount{}
	}

	if err := h.renderer.Render(w, "transaction/list", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleNewPage renders GET /transactions/new.
func (h *TransactionWebHandler) HandleNewPage(w http.ResponseWriter, r *http.Request) {
	accounts, _ := h.accountSvc.List(r.Context(), 0, 0)
	symbols, _ := h.symbolSvc.List(r.Context(), 0, 0)

	data := newTransactionFormPageData(web.PageData{
		Title: "New Transaction",
	}, accounts, symbols, "/transactions", "Create Transaction", "/transactions")

	if err := h.renderer.Render(w, "transaction/form", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleCreatePage handles POST /transactions (form submission).
func (h *TransactionWebHandler) HandleCreatePage(w http.ResponseWriter, r *http.Request) {
	accounts, _ := h.accountSvc.List(r.Context(), 0, 0)
	symbols, _ := h.symbolSvc.List(r.Context(), 0, 0)

	accountID, _ := strconv.ParseInt(r.FormValue("account_id"), 10, 64)
	date := r.FormValue("date")
	txType := r.FormValue("type")
	symbol := strings.TrimSpace(r.FormValue("symbol"))
	quantityStr := r.FormValue("quantity")
	priceStr := r.FormValue("price")
	currency := r.FormValue("currency")
	netCashStr := r.FormValue("net_cash")
	externalSystem := r.FormValue("external_system")
	externalRef := r.FormValue("external_reference")

	quantity, err := decimal.Parse(quantityStr)
	if err != nil {
		quantity = decimal.Zero
	}
	price, err := decimal.Parse(priceStr)
	if err != nil {
		price = decimal.Zero
	}
	netCash, err := decimal.Parse(netCashStr)
	if err != nil {
		netCash = decimal.Zero
	}
	description := strings.TrimSpace(r.FormValue("description"))
	lotID := strings.TrimSpace(r.FormValue("lot_id"))

	var extSystem, extRef *string
	if externalSystem != "" {
		extSystem = &externalSystem
	}
	if externalRef != "" {
		extRef = &externalRef
	}
	var lotIDPtr *string
	if lotID != "" {
		lotIDPtr = &lotID
	}
	var descPtr *string
	if description != "" {
		descPtr = &description
	}

	req := transaction.CreateRequest{
		AccountID:         accountID,
		Date:              date,
		Type:              txType,
		Symbol:            symbol,
		Quantity:          quantity,
		Price:             price,
		Currency:          currency,
		NetCash:           netCash,
		Description:       descPtr,
		LotID:             lotIDPtr,
		ExternalSystem:    extSystem,
		ExternalReference: extRef,
	}

	// Pre-validate required fields before calling the service
	if r.FormValue("account_id") == "" {
		h.renderCreateFormError(w, r, accounts, symbols, map[string]string{
			"account_id": "Account is required",
		})
		return
	}

	t, err := h.transactionSvc.Create(r.Context(), req)
	if err != nil {
		data := newTransactionFormPageData(web.PageData{
			Title: "New Transaction",
			Error: transactionUserFriendlyError(err),
		}, accounts, symbols, "/transactions", "Create Transaction", "/transactions")
		data.AccountID = r.FormValue("account_id")
		data.Date = date
		data.Type = txType
		data.Symbol = r.FormValue("symbol")
		data.Quantity = quantityStr
		data.Price = priceStr
		data.Currency = currency
		data.NetCash = netCashStr
		data.Description = r.FormValue("description")
		data.LotID = r.FormValue("lot_id")
		data.ExternalSystem = externalSystem
		data.ExternalRef = externalRef

		// Set field-level errors
		data.FieldErrors = mapFieldErrors(err, req)

		if renderErr := h.renderer.Render(w, "transaction/form", data); renderErr != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		return
	}

	setFlash(w, "Transaction created successfully")
	http.Redirect(w, r, "/transactions/"+strconv.FormatInt(t.ID, 10), http.StatusSeeOther)
}

// renderCreateFormError renders the create form with pre-validation errors
// before calling the service (e.g., empty required fields).
func (h *TransactionWebHandler) renderCreateFormError(w http.ResponseWriter, r *http.Request, accounts []account.Account, symbols []symbolmapping.SymbolMapping, fieldErrors map[string]string) {
	data := newTransactionFormPageData(web.PageData{
		Title: "New Transaction",
	}, accounts, symbols, "/transactions", "Create Transaction", "/transactions")
	data.AccountID = r.FormValue("account_id")
	data.Date = r.FormValue("date")
	data.Type = r.FormValue("type")
	data.Symbol = r.FormValue("symbol")
	data.Quantity = r.FormValue("quantity")
	data.Price = r.FormValue("price")
	data.Currency = r.FormValue("currency")
	data.NetCash = r.FormValue("net_cash")
	data.Description = r.FormValue("description")
	data.LotID = r.FormValue("lot_id")
	data.ExternalSystem = r.FormValue("external_system")
	data.ExternalRef = r.FormValue("external_reference")
	data.FieldErrors = fieldErrors

	if renderErr := h.renderer.Render(w, "transaction/form", data); renderErr != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// HandleDetailPage renders GET /transactions/{id}.
func (h *TransactionWebHandler) HandleDetailPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	t, err := h.transactionSvc.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Get account name
	accounts, _ := h.accountSvc.List(r.Context(), 0, 0)
	accountName := ""
	for _, a := range accounts {
		if a.ID == t.AccountID {
			accountName = a.Name
			break
		}
	}

	data := struct {
		web.PageData
		Transaction   transaction.Transaction
		AccountName   string
	}{
		PageData: web.PageData{
			Title: "Transaction #" + strconv.FormatInt(t.ID, 10),
			Flash: getFlash(w, r),
		},
		Transaction: *t,
		AccountName: accountName,
	}

	if err := h.renderer.Render(w, "transaction/detail", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleEditPage renders GET /transactions/{id}/edit.
func (h *TransactionWebHandler) HandleEditPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	t, err := h.transactionSvc.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	accounts, _ := h.accountSvc.List(r.Context(), 0, 0)
	symbols, _ := h.symbolSvc.List(r.Context(), 0, 0)

	editAction := "/transactions/" + strconv.FormatInt(id, 10) + "/edit"
	cancelHref := "/transactions/" + strconv.FormatInt(id, 10)

	data := newTransactionFormPageData(web.PageData{
		Title: "Edit Transaction",
	}, accounts, symbols, editAction, "Save Changes", cancelHref)
	data.Transaction = t
	data.AccountID = strconv.FormatInt(t.AccountID, 10)
	data.Date = t.Date.Format("2006-01-02")
	data.Type = t.Type
	data.Symbol = t.Symbol
	data.Quantity = t.Quantity.String()
	data.Price = t.Price.String()
	data.Currency = t.Currency
	data.NetCash = t.NetCash.String()
	if t.Description != nil {
		data.Description = *t.Description
	}
	if t.LotID != nil {
		data.LotID = *t.LotID
	}
	if t.ExternalSystem != nil {
		data.ExternalSystem = *t.ExternalSystem
	}
	if t.ExternalReference != nil {
		data.ExternalRef = *t.ExternalReference
	}

	if err := h.renderer.Render(w, "transaction/form", data); err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// HandleEditPost handles POST /transactions/{id}/edit.
func (h *TransactionWebHandler) HandleEditPost(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Get current transaction for comparison
	current, err := h.transactionSvc.Get(r.Context(), id)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	accounts, _ := h.accountSvc.List(r.Context(), 0, 0)
	symbols, _ := h.symbolSvc.List(r.Context(), 0, 0)

	date := r.FormValue("date")
	txType := r.FormValue("type")
	symbol := strings.TrimSpace(r.FormValue("symbol"))
	quantityStr := r.FormValue("quantity")
	priceStr := r.FormValue("price")
	currency := r.FormValue("currency")
	netCashStr := r.FormValue("net_cash")
	externalSystem := r.FormValue("external_system")
	externalRef := r.FormValue("external_reference")

	quantity, err := decimal.Parse(quantityStr)
	if err != nil {
		quantity = decimal.Zero
	}
	price, err := decimal.Parse(priceStr)
	if err != nil {
		price = decimal.Zero
	}
	netCash, err := decimal.Parse(netCashStr)
	if err != nil {
		netCash = decimal.Zero
	}
	description := strings.TrimSpace(r.FormValue("description"))
	lotID := strings.TrimSpace(r.FormValue("lot_id"))

	req := transaction.UpdateRequest{}

	// Only include fields that were actually changed
	if date != "" && date != current.Date.Format("2006-01-02") {
		req.Date = &date
	}
	if txType != current.Type {
		req.Type = &txType
	}
	if symbol != current.Symbol {
		req.Symbol = &symbol
	}
	if !quantity.Equal(current.Quantity) {
		req.Quantity = &quantity
	}
	if !price.Equal(current.Price) {
		req.Price = &price
	}
	if currency != current.Currency {
		req.Currency = &currency
	}
	if !netCash.Equal(current.NetCash) {
		req.NetCash = transaction.OptionalDecimal{Dec: netCash, IsSet: true}
	}
	if (description != "" && current.Description == nil) ||
		(description == "" && current.Description != nil) ||
		(description != "" && current.Description != nil && description != *current.Description) {
		req.Description = &description
	}
	if lotID != "" && (current.LotID == nil || *current.LotID != lotID) {
		req.LotID = &lotID
	}
	var extSystem, extRef *string
	if externalSystem != "" {
		extSystem = &externalSystem
	}
	if externalRef != "" {
		extRef = &externalRef
	}
	if (extSystem != nil && current.ExternalSystem == nil) ||
		(extSystem == nil && current.ExternalSystem != nil) ||
		(extSystem != nil && current.ExternalSystem != nil && *extSystem != *current.ExternalSystem) {
		req.ExternalSystem = extSystem
	}
	if (extRef != nil && current.ExternalReference == nil) ||
		(extRef == nil && current.ExternalReference != nil) ||
		(extRef != nil && current.ExternalReference != nil && *extRef != *current.ExternalReference) {
		req.ExternalReference = extRef
	}

	t, err := h.transactionSvc.Update(r.Context(), id, req)
	if err != nil {
		editAction := "/transactions/" + strconv.FormatInt(id, 10) + "/edit"
		cancelHref := "/transactions/" + strconv.FormatInt(id, 10)

		data := newTransactionFormPageData(web.PageData{
			Title: "Edit Transaction",
			Error: transactionUserFriendlyError(err),
		}, accounts, symbols, editAction, "Save Changes", cancelHref)
		data.AccountID = r.FormValue("account_id")
		data.Date = date
		data.Type = txType
		data.Symbol = r.FormValue("symbol")
		data.Quantity = quantityStr
		data.Price = priceStr
		data.Currency = currency
		data.NetCash = netCashStr
		data.Description = r.FormValue("description")
		data.LotID = r.FormValue("lot_id")
		data.ExternalSystem = externalSystem
		data.ExternalRef = externalRef

		// Set field-level errors
		data.FieldErrors = mapFieldErrors(err, transaction.CreateRequest{
			Date:     date,
			Type:     txType,
			Symbol:   symbol,
			Quantity: quantity,
			Price:    price,
			Currency: currency,
			NetCash:  netCash,
			LotID:    strPtr(lotID),
		})

		if renderErr := h.renderer.Render(w, "transaction/form", data); renderErr != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		return
	}

	setFlash(w, "Transaction updated successfully")
	http.Redirect(w, r, "/transactions/"+strconv.FormatInt(t.ID, 10), http.StatusSeeOther)
}

// HandleDeletePage handles POST /transactions/{id}/delete.
func (h *TransactionWebHandler) HandleDeletePage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := h.transactionSvc.Delete(r.Context(), id); err != nil {
		if errors.Is(err, transaction.ErrNotFound) {
			setFlash(w, "Transaction not found")
			http.Redirect(w, r, "/transactions", http.StatusSeeOther)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	setFlash(w, "Transaction deleted successfully")
	http.Redirect(w, r, "/transactions", http.StatusSeeOther)
}

// parseTransactionFilter extracts filter parameters from query string.
func parseTransactionFilter(query url.Values) TransactionFilter {
	return TransactionFilter{
		AccountID: query.Get("account_id"),
		Symbol:    query.Get("symbol"),
		Type:      query.Get("type"),
		DateFrom:  query.Get("date_from"),
		DateTo:    query.Get("date_to"),
	}
}

// transactionUserFriendlyError returns a user-friendly message from a transaction service error.
func transactionUserFriendlyError(err error) string {
	if errors.Is(err, transaction.ErrInvalidType) {
		return "Invalid transaction type"
	}
	if errors.Is(err, transaction.ErrInvalidQuantity) {
		return "Quantity must be non-zero"
	}
	if errors.Is(err, transaction.ErrInvalidPrice) {
		return "Price must be greater than zero"
	}
	if errors.Is(err, transaction.ErrInvalidCurrency) {
		return "Invalid currency code (must be 3-letter ISO 4217, e.g. USD)"
	}
	if errors.Is(err, transaction.ErrInvalidDate) {
		return "Invalid date format (expected YYYY-MM-DD)"
	}
	if errors.Is(err, transaction.ErrInvalidSymbol) {
		return "Symbol is required"
	}
	if errors.Is(err, transaction.ErrSymbolNotFound) {
		return "Symbol not found in symbol map"
	}
	if errors.Is(err, transaction.ErrInvalidNetCash) {
		return "Net cash is required and must be non-zero"
	}
	if errors.Is(err, transaction.ErrAccountNotFound) {
		return "Selected account not found"
	}
	if errors.Is(err, transaction.ErrInvalidExternalField) {
		return "External system/reference must be at most 100 characters"
	}
	if errors.Is(err, transaction.ErrLotNotFound) {
		return "Referenced lot not found"
	}
	if errors.Is(err, transaction.ErrLotSymbolMismatch) {
		return "Lot belongs to a different symbol"
	}
	if errors.Is(err, transaction.ErrLotAccountMismatch) {
		return "Lot belongs to a different account"
	}
	if errors.Is(err, transaction.ErrLotTypeMismatch) {
		return "Lot type does not match transaction type"
	}
	if errors.Is(err, transaction.ErrInvalidLotID) {
		return "Invalid lot ID (must be at most 100 characters, and immutable once set)"
	}
	if errors.Is(err, transaction.ErrNotFound) {
		return "Transaction not found"
	}
	return "An error occurred. Please try again."
}

// mapFieldErrors maps a service error to per-field validation errors.
func mapFieldErrors(err error, req transaction.CreateRequest) map[string]string {
	if err == nil {
		return nil
	}
	fieldErrors := make(map[string]string)
	switch {
	case errors.Is(err, transaction.ErrInvalidDate):
		fieldErrors["date"] = "Invalid date format (expected YYYY-MM-DD)"
	case errors.Is(err, transaction.ErrInvalidType):
		fieldErrors["type"] = "Invalid transaction type"
	case errors.Is(err, transaction.ErrInvalidSymbol):
		fieldErrors["symbol"] = "Symbol is required"
	case errors.Is(err, transaction.ErrSymbolNotFound):
		fieldErrors["symbol"] = "Symbol not found in symbol map"
	case errors.Is(err, transaction.ErrInvalidQuantity):
		fieldErrors["quantity"] = "Quantity must be non-zero"
	case errors.Is(err, transaction.ErrInvalidPrice):
		fieldErrors["price"] = "Price must be greater than zero"
	case errors.Is(err, transaction.ErrInvalidCurrency):
		fieldErrors["currency"] = "Invalid currency code (must be 3-letter ISO 4217, e.g. USD)"
	case errors.Is(err, transaction.ErrInvalidNetCash):
		fieldErrors["net_cash"] = "Net cash is required and must be non-zero"
	case errors.Is(err, transaction.ErrAccountNotFound):
		fieldErrors["account_id"] = "Selected account not found"
	case errors.Is(err, transaction.ErrInvalidExternalField):
		fieldErrors["external_system"] = "External system/reference must be at most 100 characters"
	case errors.Is(err, transaction.ErrLotNotFound):
		fieldErrors["lot_id"] = "Referenced lot not found"
	case errors.Is(err, transaction.ErrLotSymbolMismatch):
		fieldErrors["lot_id"] = "Lot belongs to a different symbol"
	case errors.Is(err, transaction.ErrLotAccountMismatch):
		fieldErrors["lot_id"] = "Lot belongs to a different account"
	case errors.Is(err, transaction.ErrLotTypeMismatch):
		fieldErrors["lot_id"] = "Lot type does not match transaction type"
	case errors.Is(err, transaction.ErrInvalidLotID):
		fieldErrors["lot_id"] = "Invalid lot ID"
	}
	return fieldErrors
}

// strPtr returns a pointer to a string.
func strPtr(s string) *string {
	return &s
}
