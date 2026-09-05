package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/eddiectc/portfoliolab/internal/domain/brokerimport"
	"github.com/eddiectc/portfoliolab/internal/domain/ibkrimport"
	"github.com/eddiectc/portfoliolab/internal/domain/symbolmapping"
)

// ImportService defines the import operations needed by the HTTP handler.
type ImportService interface {
	Preview(ctx context.Context, data []byte, accountID int64) (*brokerimport.PreviewResponse, error)
	ConfirmImport(ctx context.Context, data []byte, accountID int64) (*brokerimport.ImportResult, error)
	AddBrokerSymbolMapping(ctx context.Context, brokerName, brokerSymbol, internalSymbol string) error
}

// ImportHandler handles HTTP requests for IBKR Flex XML import.
type ImportHandler struct {
	importSvc   ImportService
	symbolSvc   SymbolService
	maxFileSize int64
}

// NewImportHandler creates a new IBKR import HTTP handler.
func NewImportHandler(importSvc ImportService, symbolSvc SymbolService) *ImportHandler {
	return &ImportHandler{
		importSvc:   importSvc,
		symbolSvc:   symbolSvc,
		maxFileSize: 50 << 20, // 50 MB
	}
}

// RegisterRoutes mounts IBKR import routes on the given router.
func (h *ImportHandler) RegisterRoutes(r *chi.Mux) {
	r.Post("/api/transactions/import/ibkr/preview", h.HandlePreview)
	r.Post("/api/transactions/import/ibkr/confirm", h.HandleConfirm)
	r.Post("/api/transactions/import/ibkr/symbols", h.HandleCreateSymbol)
	r.Post("/api/transactions/import/ibkr/broker-symbols", h.HandleAddBrokerSymbol)
}

// HandlePreview handles POST /api/transactions/import/ibkr/preview.
// Accepts multipart form with XML file and account_id.
func (h *ImportHandler) HandlePreview(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(h.maxFileSize); err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_REQUEST", "file too large or invalid form data")
		return
	}

	accountID, err := parseAccountID(r.FormValue("account_id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ACCOUNT_ID", "invalid account_id")
		return
	}

	xmlData, err := readXMLFile(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_XML_FILE", err.Error())
		return
	}

	preview, err := h.importSvc.Preview(r.Context(), xmlData, accountID)
	if err != nil {
		h.handleImportError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, preview)
}

// HandleConfirm handles POST /api/transactions/import/ibkr/confirm.
// Accepts multipart form with XML file and account_id.
func (h *ImportHandler) HandleConfirm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(h.maxFileSize); err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_REQUEST", "file too large or invalid form data")
		return
	}

	accountID, err := parseAccountID(r.FormValue("account_id"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_ACCOUNT_ID", "invalid account_id")
		return
	}

	xmlData, err := readXMLFile(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "INVALID_XML_FILE", err.Error())
		return
	}

	result, err := h.importSvc.ConfirmImport(r.Context(), xmlData, accountID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "IMPORT_FAILED", "failed to import transactions")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// HandleCreateSymbol handles POST /api/transactions/import/ibkr/symbols.
// Accepts JSON body with internal_symbol and market_data_symbol.
func (h *ImportHandler) HandleCreateSymbol(w http.ResponseWriter, r *http.Request) {
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

// HandleAddBrokerSymbol handles POST /api/transactions/import/ibkr/broker-symbols.
// Accepts JSON body with broker_name, broker_symbol, and internal_symbol.
func (h *ImportHandler) HandleAddBrokerSymbol(w http.ResponseWriter, r *http.Request) {
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

func (h *ImportHandler) handleImportError(w http.ResponseWriter, err error) {
	if errors.Is(err, ibkrimport.ErrAccountNotFound) {
		writeJSONError(w, http.StatusNotFound, "ACCOUNT_NOT_FOUND", "account not found")
		return
	}
	if errors.Is(err, ibkrimport.ErrInvalidXML) {
		writeJSONError(w, http.StatusBadRequest, "INVALID_XML", err.Error())
		return
	}
	writeJSONError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
}

func (h *ImportHandler) handleSymbolError(w http.ResponseWriter, err error) {
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

// parseAccountID parses an account ID from a form value.
func parseAccountID(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("account_id is required")
	}
	return strconv.ParseInt(s, 10, 64)
}

// readXMLFile reads the XML file from a multipart form.
func readXMLFile(r *http.Request) ([]byte, error) {
	file, header, err := r.FormFile("xml_file")
	if err != nil {
		return nil, errors.New("xml_file is required")
	}
	defer func() { _ = file.Close() }()

	if header.Size == 0 {
		return nil, errors.New("xml_file is empty")
	}

	return io.ReadAll(file)
}
