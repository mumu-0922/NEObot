package httpserver

import (
	"log/slog"
	"net/http"
)

type Middleware func(http.Handler) http.Handler

func chain(handler http.Handler, middlewares ...Middleware) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}

	return handler
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func withRecover(loggers ...*slog.Logger) Middleware {
	var logger *slog.Logger
	if len(loggers) > 0 {
		logger = loggers[0]
	}
	if logger == nil {
		logger = slog.Default()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.ErrorContext(
						r.Context(),
						"http_panic",
						slog.String("request_id", RequestIDFromContext(r.Context())),
						slog.String("panic_type", panicType(recovered)),
					)
					writeJSON(w, http.StatusInternalServerError, ErrorResponse{
						Error: ErrorBody{
							Code:    "INTERNAL_ERROR",
							Message: "internal server error",
						},
					})
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

func panicType(recovered any) string {
	if recovered == nil {
		return "<nil>"
	}

	switch recovered.(type) {
	case string:
		return "string"
	case error:
		return "error"
	default:
		return "unknown"
	}
}
