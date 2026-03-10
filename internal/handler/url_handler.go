package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/emsbt/url-shortener/internal/model"
	"github.com/emsbt/url-shortener/internal/service"
	"github.com/go-chi/chi/v5"
)

// URLHandler contém os handlers HTTP para encurtamento de URLs.
type URLHandler struct {
	svc    service.URLService
	logger *slog.Logger
}

// NewURLHandler cria um novo URLHandler.
func NewURLHandler(svc service.URLService, logger *slog.Logger) *URLHandler {
	return &URLHandler{svc: svc, logger: logger}
}

// CreateURL trata requisições POST /v1/urls.
func (h *URLHandler) CreateURL(w http.ResponseWriter, r *http.Request) {
	var req model.CreateURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON request body")
		return
	}

	resp, err := h.svc.Create(r.Context(), &req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidURL):
			writeError(w, http.StatusBadRequest, "INVALID_URL", err.Error())
		case errors.Is(err, service.ErrAliasConflict):
			writeError(w, http.StatusConflict, "ALIAS_CONFLICT", "The requested alias is already in use")
		case errors.Is(err, service.ErrDuplicateURL):
			writeError(w, http.StatusConflict, "URL_ALREADY_EXISTS", "This URL has already been registered")
		default:
			h.logger.ErrorContext(r.Context(), "create url error", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "An unexpected error occurred")
		}
		return
	}

	writeJSON(w, http.StatusCreated, resp)
}

// GetURL trata requisições GET /v1/urls/{id}.
func (h *URLHandler) GetURL(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	resp, err := h.svc.GetByID(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrURLNotFound):
			writeError(w, http.StatusNotFound, "URL_NOT_FOUND", "The requested short URL does not exist")
		default:
			h.logger.ErrorContext(r.Context(), "get url error", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "An unexpected error occurred")
		}
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// ListURLs trata requisições GET /v1/urls?page=1&size=10.
func (h *URLHandler) ListURLs(w http.ResponseWriter, r *http.Request) {
	page := queryInt(r, "page", 1)
	size := queryInt(r, "size", 10)

	resp, err := h.svc.List(r.Context(), page, size)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "list urls error", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "An unexpected error occurred")
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// UpdateURL trata requisições PATCH /v1/urls/{id}.
func (h *URLHandler) UpdateURL(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req model.UpdateURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON request body")
		return
	}

	resp, err := h.svc.Update(r.Context(), id, &req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrURLNotFound):
			writeError(w, http.StatusNotFound, "URL_NOT_FOUND", "The requested short URL does not exist")
		case errors.Is(err, service.ErrInvalidURL):
			writeError(w, http.StatusBadRequest, "INVALID_URL", err.Error())
		case errors.Is(err, service.ErrDuplicateURL):
			writeError(w, http.StatusConflict, "URL_ALREADY_EXISTS", "This URL has already been registered")
		default:
			h.logger.ErrorContext(r.Context(), "update url error", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "An unexpected error occurred")
		}
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// DeleteURL trata requisições DELETE /v1/urls/{id}.
func (h *URLHandler) DeleteURL(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.svc.Delete(r.Context(), id); err != nil {
		switch {
		case errors.Is(err, service.ErrURLNotFound):
			writeError(w, http.StatusNotFound, "URL_NOT_FOUND", "The requested short URL does not exist")
		default:
			h.logger.ErrorContext(r.Context(), "delete url error", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "An unexpected error occurred")
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RedirectURL trata requisições GET /{id} e realiza o redirecionamento HTTP 301.
func (h *URLHandler) RedirectURL(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	target, err := h.svc.Redirect(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrURLNotFound):
			writeError(w, http.StatusNotFound, "URL_NOT_FOUND", "The requested short URL does not exist")
		case errors.Is(err, service.ErrURLExpired):
			writeError(w, http.StatusGone, "URL_EXPIRED", "The requested short URL has expired")
		default:
			h.logger.ErrorContext(r.Context(), "redirect error", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "An unexpected error occurred")
		}
		return
	}

	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

// ---- funções auxiliares ----

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Cabeçalhos de resposta já enviados; nada mais a fazer.
		return
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, model.ErrorResponse{
		Error: model.ErrorDetail{Code: code, Message: message},
	})
}

func queryInt(r *http.Request, key string, defaultVal int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 1 {
		return defaultVal
	}
	return v
}
