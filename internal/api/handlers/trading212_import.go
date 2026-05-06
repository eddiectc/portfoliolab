package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/symbolmapping"
	"github.com/arch-portfolio-lab/portfoliolab/internal/domain/trading212import"
)

// Trading212ImportHandler handles HTTP requests for Trading 212 CSV import.
type Trading212ImportHandler struct {
	importSvc   ImportService
	symbolSvc   SymbolService
	maxFileSize int64
}

// NewTrading212ImportHandler creates a new Trading 212 import HTTP handler.
func NewTrading212ImportHandler(importSvc ImportService, symbolSvc SymbolService) *Trading212ImportHandler {
	return &Trading212ImportHandler{
		importSvc:   importSvc,
		symbolSvc:   symbolSvc,
		maxFileSize: 50 << 20, // 50 MB
	}
}

// RegisterRoutes mounts Trading 212 import routes on the given router.
func (h *Trading212ImportHandler) RegisterRoutes(r *chi.Mux) {
	r.Post("/api/transactions/import/trading212/preview", h.HandlePreview)
	r.Post("/api/transactions/import/trading212/confirm", h.HandleConfirm)
	r.Post("/api/transactions/import/trading212/symbols", h.HandleCreateSymbol)
	r.Post("/api/transactions/import/trading212/broker-symbols", h.HandleAddBrokerSymbol)
}

// HandlePreview handles POST /api/transactions/import/trading212/preview.
// Accepts multipart form with CSV file and account_id.
func (h *Trading212ImportHandler) HandlePreview(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(h.maxFileSize); err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_REQUEST", "file too large or invalid form data")
		return
	}

	accountID, err := parseAccountID(r.FormValue("account_id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ACCOUNT_ID", "invalid account_id")
		return
	}

	csvData, err := readCSVFile(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_CSV_FILE", err.Error())
		return
	}

	preview, err := h.importSvc.Preview(r.Context(), csvData, accountID)
	if err != nil {
		h.handleImportError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, preview)
}

// HandleConfirm handles POST /api/transactions/import/trading212/confirm.
// Accepts multipart form with CSV file and account_id.
func (h *Trading212ImportHandler) HandleConfirm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(h.maxFileSize); err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_REQUEST", "file too large or invalid form data")
		return
	}

	accountID, err := parseAccountID(r.FormValue("account_id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ACCOUNT_ID", "invalid account_id")
		return
	}

	csvData, err := readCSVFile(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_CSV_FILE", err.Error())
		return
	}

	result, err := h.importSvc.ConfirmImport(r.Context(), csvData, accountID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "IMPORT_FAILED", "failed to import transactions")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// HandleCreateSymbol handles POST /api/transactions/import/trading212/symbols.
// Accepts JSON body with internal_symbol and market_data_symbol.
func (h *Trading212ImportHandler) HandleCreateSymbol(w http.ResponseWriter, r *http.Request) {
	var req struct {
		InternalSymbol   string `json:"internal_symbol"`
		MarketDataSymbol string `json:"market_data_symbol"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body: "+err.Error())
		return
	}

	sm, err := h.symbolSvc.Create(r.Context(), symbolmapping.CreateRequest{
		InternalSymbol:   req.InternalSymbol,
		MarketDataSymbol: req.MarketDataSymbol,
	})
	if err != nil {
		h.handleSymbolError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, sm)
}

// HandleAddBrokerSymbol handles POST /api/transactions/import/trading212/broker-symbols.
// Accepts JSON body with broker_name, broker_symbol, and internal_symbol.
func (h *Trading212ImportHandler) HandleAddBrokerSymbol(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BrokerName     string `json:"broker_name"`
		BrokerSymbol   string `json:"broker_symbol"`
		InternalSymbol string `json:"internal_symbol"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body: "+err.Error())
		return
	}

	if err := h.importSvc.AddBrokerSymbolMapping(r.Context(), req.BrokerName, req.BrokerSymbol, req.InternalSymbol); err != nil {
		if errors.Is(err, symbolmapping.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "SYMBOL_NOT_FOUND", "internal symbol not found")
			return
		}
		if errors.Is(err, symbolmapping.ErrBrokerSymbolExists) {
			writeJSONError(w, http.StatusConflict, "BROKER_SYMBOL_EXISTS", "this broker symbol is already mapped to a different internal symbol")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to add broker symbol mapping")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Trading212ImportHandler) handleImportError(w http.ResponseWriter, err error) {
	if errors.Is(err, trading212import.ErrAccountNotFound) {
		writeJSONError(w, http.StatusNotFound, "ACCOUNT_NOT_FOUND", "account not found")
		return
	}
	if errors.Is(err, trading212import.ErrInvalidCSV) {
		writeJSONError(w, http.StatusBadRequest, "INVALID_CSV", err.Error())
		return
	}
	writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}

func (h *Trading212ImportHandler) handleSymbolError(w http.ResponseWriter, err error) {
	if errors.Is(err, symbolmapping.ErrInvalidSymbol) {
		writeJSONError(w, http.StatusBadRequest, "INVALID_SYMBOL", err.Error())
		return
	}
	if errors.Is(err, symbolmapping.ErrInternalSymbolExists) {
		writeJSONError(w, http.StatusConflict, "INTERNAL_SYMBOL_EXISTS", "a symbol mapping with this internal symbol already exists")
		return
	}
	writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}

// readCSVFile reads the CSV file from a multipart form.
func readCSVFile(r *http.Request) ([]byte, error) {
	file, header, err := r.FormFile("csv_file")
	if err != nil {
		return nil, errors.New("csv_file is required")
	}
	defer file.Close()

	if header.Size == 0 {
		return nil, errors.New("csv_file is empty")
	}

	return io.ReadAll(file)
}


