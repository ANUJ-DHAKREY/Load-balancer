package proxy

import (
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/anujdhakrey/load-balancer/internal/circuitbreaker"
	"github.com/anujdhakrey/load-balancer/internal/pool"
	"github.com/anujdhakrey/load-balancer/internal/ratelimiter"
	"github.com/anujdhakrey/load-balancer/internal/strategy"
)

type LoadBalancer struct {
	pool       *pool.Pool
	strategy   strategy.Strategy
	limiter    *ratelimiter.RateLimiter
	breakers   map[string]*circuitbreaker.CircuitBreaker
	maxRetries int
	logger     *slog.Logger
}

func New(
	p *pool.Pool,
	strat strategy.Strategy,
	limiter *ratelimiter.RateLimiter,
	maxRetries int,
	logger *slog.Logger,
) *LoadBalancer {
	lb := &LoadBalancer{
		pool:       p,
		strategy:   strat,
		limiter:    limiter,
		breakers:   make(map[string]*circuitbreaker.CircuitBreaker),
		maxRetries: maxRetries,
		logger:     logger,
	}

	// Initialize circuit breakers for existing backends.
	for _, b := range p.Backends() {
		lb.breakers[b.URL.String()] = circuitbreaker.New(5, 3, 30*time.Second)
	}

	return lb
}

func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Rate limiting.
	if lb.limiter != nil {
		ip := extractClientIP(r)
		if !lb.limiter.Allow(ip) {
			http.Error(w, `{"status":"error","message":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}
	}

	// Try to find a healthy backend.
	var lastErr error
	attempted := make(map[string]bool)

	for retry := 0; retry <= lb.maxRetries; retry++ {
		backends := lb.pool.HealthyBackends()
		if len(backends) == 0 {
			http.Error(w, `{"status":"error","message":"no healthy backends available"}`, http.StatusServiceUnavailable)
			return
		}

		target := lb.strategy.Next(backends)
		if target == nil {
			http.Error(w, `{"status":"error","message":"no available backend"}`, http.StatusServiceUnavailable)
			return
		}

		backendURL := target.URL.String()

		// Skip already-attempted backends on retries (best effort).
		if retry > 0 && attempted[backendURL] && len(backends) > 1 {
			// Try to find a different one.
			found := false
			for _, b := range backends {
				if !attempted[b.URL.String()] {
					target = b
					backendURL = b.URL.String()
					found = true
					break
				}
			}
			if !found {
				// All backends attempted, retry anyway.
				target = lb.strategy.Next(backends)
				if target == nil {
					break
				}
				backendURL = target.URL.String()
			}
		}

		attempted[backendURL] = true

		// Circuit breaker check.
		cb := lb.getBreaker(backendURL)
		if !cb.Allow() {
			lb.logger.Debug("circuit breaker open, skipping", "backend", backendURL)
			lastErr = fmt.Errorf("circuit breaker open for %s", backendURL)
			continue
		}

		// Track active connections.
		target.IncrementConnections()
		start := time.Now()

		// Configure the reverse proxy error handler to capture failures.
		recorder := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		target.Proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
			lb.logger.Error("proxy error", "backend", backendURL, "error", err)
			recorder.proxyErr = err
		}

		// Set forwarding headers.
		r.Header.Set("X-Forwarded-For", extractClientIP(r))
		r.Header.Set("X-Forwarded-Host", r.Host)
		r.Header.Set("X-Forwarded-Proto", schemeFromRequest(r))
		r.Header.Set("X-Real-IP", extractClientIP(r))

		target.Proxy.ServeHTTP(recorder, r)

		latency := time.Since(start).Milliseconds()
		target.DecrementConnections()

		if recorder.proxyErr != nil {
			target.RecordRequest(latency, true)
			cb.RecordFailure()
			lastErr = recorder.proxyErr
			lb.logger.Warn("retrying request",
				"attempt", retry+1,
				"backend", backendURL,
				"error", recorder.proxyErr,
			)
			continue
		}

		// Successful proxy.
		failed := recorder.statusCode >= 500
		target.RecordRequest(latency, failed)
		if failed {
			cb.RecordFailure()
		} else {
			cb.RecordSuccess()
		}

		lb.logger.Debug("request proxied",
			"method", r.Method,
			"path", r.URL.Path,
			"backend", backendURL,
			"status", recorder.statusCode,
			"latency_ms", latency,
		)
		return
	}

	// All retries exhausted.
	lb.logger.Error("all retries exhausted", "error", lastErr)
	http.Error(w, `{"status":"error","message":"all backends failed"}`, http.StatusBadGateway)
}

func (lb *LoadBalancer) getBreaker(backendURL string) *circuitbreaker.CircuitBreaker {
	cb, ok := lb.breakers[backendURL]
	if !ok {
		cb = circuitbreaker.New(5, 3, 30*time.Second)
		lb.breakers[backendURL] = cb
	}
	return cb
}

func extractClientIP(r *http.Request) string {
	// Check X-Forwarded-For first (first IP in chain).
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		ip := strings.TrimSpace(parts[0])
		if ip != "" {
			return ip
		}
	}

	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func schemeFromRequest(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

// responseRecorder captures the status code and any proxy errors.
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	proxyErr   error
	written    bool
}

func (rr *responseRecorder) WriteHeader(code int) {
	if !rr.written {
		rr.statusCode = code
		rr.written = true
	}
	rr.ResponseWriter.WriteHeader(code)
}

func (rr *responseRecorder) Write(b []byte) (int, error) {
	if !rr.written {
		rr.statusCode = http.StatusOK
		rr.written = true
	}
	return rr.ResponseWriter.Write(b)
}
