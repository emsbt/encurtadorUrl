package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// APIKeyAuth retorna um middleware que exige que o cabeçalho X-API-Key corresponda
// à chave configurada. Se a chave estiver vazia, a autenticação é ignorada.
func APIKeyAuth(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if apiKey == "" {
				next.ServeHTTP(w, r)
				return
			}
			key := r.Header.Get("X-API-Key")
			if key != apiKey {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":{"code":"UNAUTHORIZED","message":"Invalid or missing API key"}}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequestLogger retorna um middleware que registra cada requisição recebida usando
// logging estruturado via slog.
func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(rw, r)

			logger.InfoContext(r.Context(), "request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rw.statusCode),
				slog.Duration("duration", time.Since(start)),
			)
		})
	}
}

// Recoverer retorna um middleware que recupera de panics, registra o erro e
// retorna um erro 500 Internal Server Error.
func Recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.ErrorContext(r.Context(), "panic recovered",
						slog.Any("panic", rec),
					)
					http.Error(w,
						`{"error":{"code":"INTERNAL_SERVER_ERROR","message":"An unexpected error occurred"}}`,
						http.StatusInternalServerError,
					)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// responseWriter encapsula http.ResponseWriter para capturar o código de status.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
