package api

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/lakehouse-platform/lp/internal/auth"
	"github.com/lakehouse-platform/lp/internal/store"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Logging emits one structured access-log line per request. This doubles as
// the Phase 1 audit trail until audit-svc lands.
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqID := r.Header.Get("X-Request-Id")
		if reqID == "" {
			reqID = store.NewID("req-")
		}
		w.Header().Set("X-Request-Id", reqID)
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		attrs := []any{"method", r.Method, "path", r.URL.Path, "status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(), "request_id", reqID}
		if p, ok := auth.FromContext(r.Context()); ok {
			attrs = append(attrs, "principal", p.Subject)
		}
		slog.InfoContext(r.Context(), "http", attrs...)
	})
}

func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				slog.ErrorContext(r.Context(), "panic", "value", v, "stack", string(debug.Stack()))
				WriteJSON(w, http.StatusInternalServerError, Errorf(CodeInternal, "internal error"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
