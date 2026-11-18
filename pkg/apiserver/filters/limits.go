package filters

import (
	"net/http"

	"golang.org/x/time/rate"

	apiruntime "github.com/smartcat999/game-panel-cloud/pkg/apiserver/runtime"
)

// DefaultMaxRequestBodyBytes is the default maximum HTTP request body size (4 MiB).
const DefaultMaxRequestBodyBytes int64 = 4 << 20

// WithMaxRequestBodyBytes wraps req.Body with http.MaxBytesReader to protect the API server
// from unbounded request payload memory exhaustion.
func WithMaxRequestBodyBytes(handler http.Handler, maxBytes int64) http.Handler {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxRequestBodyBytes
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Body != nil && req.ContentLength > maxBytes {
			apiruntime.WriteError(w, http.StatusRequestEntityTooLarge, "request body exceeds maximum allowed size")
			return
		}
		if req.Body != nil {
			req.Body = http.MaxBytesReader(w, req.Body, maxBytes)
		}
		handler.ServeHTTP(w, req)
	})
}

// WithRateLimit enforces token-bucket rate limiting backed by golang.org/x/time/rate.
// If rps <= 0 or burst <= 0, the handler is returned unwrapped.
func WithRateLimit(handler http.Handler, rps float64, burst int) http.Handler {
	if rps <= 0 || burst <= 0 {
		return handler
	}
	limiter := rate.NewLimiter(rate.Limit(rps), burst)
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if !limiter.Allow() {
			apiruntime.WriteError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		handler.ServeHTTP(w, req)
	})
}
