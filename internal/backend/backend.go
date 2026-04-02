package backend

import (
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

type State int

const (
	StateHealthy State = iota
	StateUnhealthy
	StateDraining
)

func (s State) String() string {
	switch s {
	case StateHealthy:
		return "healthy"
	case StateUnhealthy:
		return "unhealthy"
	case StateDraining:
		return "draining"
	default:
		return "unknown"
	}
}

type Backend struct {
	URL            *url.URL
	Weight         int
	Proxy          *httputil.ReverseProxy
	state          State
	activeConns    int64
	totalRequests  int64
	totalFailures  int64
	totalLatencyMs int64
	lastChecked    time.Time
	mu             sync.RWMutex
}

func New(rawURL string, weight int) (*Backend, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}

	proxy := httputil.NewSingleHostReverseProxy(u)

	return &Backend{
		URL:    u,
		Weight: weight,
		Proxy:  proxy,
		state:  StateHealthy,
	}, nil
}

func (b *Backend) IsAlive() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.state == StateHealthy
}

func (b *Backend) State() State {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.state
}

func (b *Backend) SetState(s State) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.state = s
	b.lastChecked = time.Now()
}

func (b *Backend) ActiveConnections() int64 {
	return atomic.LoadInt64(&b.activeConns)
}

func (b *Backend) IncrementConnections() {
	atomic.AddInt64(&b.activeConns, 1)
}

func (b *Backend) DecrementConnections() {
	atomic.AddInt64(&b.activeConns, -1)
}

func (b *Backend) RecordRequest(latencyMs int64, failed bool) {
	atomic.AddInt64(&b.totalRequests, 1)
	atomic.AddInt64(&b.totalLatencyMs, latencyMs)
	if failed {
		atomic.AddInt64(&b.totalFailures, 1)
	}
}

type Stats struct {
	URL               string  `json:"url"`
	State             string  `json:"state"`
	Weight            int     `json:"weight"`
	ActiveConnections int64   `json:"active_connections"`
	TotalRequests     int64   `json:"total_requests"`
	TotalFailures     int64   `json:"total_failures"`
	AvgLatencyMs      float64 `json:"avg_latency_ms"`
	LastChecked       string  `json:"last_checked"`
}

func (b *Backend) Stats() Stats {
	b.mu.RLock()
	defer b.mu.RUnlock()

	totalReqs := atomic.LoadInt64(&b.totalRequests)
	totalLatency := atomic.LoadInt64(&b.totalLatencyMs)
	var avgLatency float64
	if totalReqs > 0 {
		avgLatency = float64(totalLatency) / float64(totalReqs)
	}

	lastChecked := ""
	if !b.lastChecked.IsZero() {
		lastChecked = b.lastChecked.Format(time.RFC3339)
	}

	return Stats{
		URL:               b.URL.String(),
		State:             b.state.String(),
		Weight:            b.Weight,
		ActiveConnections: atomic.LoadInt64(&b.activeConns),
		TotalRequests:     totalReqs,
		TotalFailures:     atomic.LoadInt64(&b.totalFailures),
		AvgLatencyMs:      avgLatency,
		LastChecked:       lastChecked,
	}
}
