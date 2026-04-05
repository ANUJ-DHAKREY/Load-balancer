package health

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/anujdhakrey/load-balancer/internal/backend"
	"github.com/anujdhakrey/load-balancer/internal/pool"
)

type Checker struct {
	pool     *pool.Pool
	interval time.Duration
	timeout  time.Duration
	path     string
	client   *http.Client
	logger   *slog.Logger
}

func NewChecker(p *pool.Pool, interval, timeout time.Duration, path string, logger *slog.Logger) *Checker {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: timeout,
		}).DialContext,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     30 * time.Second,
	}

	return &Checker{
		pool:     p,
		interval: interval,
		timeout:  timeout,
		path:     path,
		client: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
		logger: logger,
	}
}

func (c *Checker) Run(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	// Run an initial check immediately.
	c.checkAll()

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("health checker stopped")
			return
		case <-ticker.C:
			c.checkAll()
		}
	}
}

func (c *Checker) checkAll() {
	backends := c.pool.Backends()
	for _, b := range backends {
		if b.State() == backend.StateDraining {
			continue
		}
		go c.check(b)
	}
}

func (c *Checker) check(b *backend.Backend) {
	checkURL := b.URL.JoinPath(c.path).String()

	req, err := http.NewRequest(http.MethodGet, checkURL, nil)
	if err != nil {
		c.logger.Error("failed to create health check request", "url", checkURL, "error", err)
		b.SetState(backend.StateUnhealthy)
		return
	}
	req.Header.Set("User-Agent", "LoadBalancer-HealthCheck/1.0")

	resp, err := c.client.Do(req)
	if err != nil {
		if b.IsAlive() {
			c.logger.Warn("backend became unhealthy", "url", b.URL.String(), "error", err)
		}
		b.SetState(backend.StateUnhealthy)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		if !b.IsAlive() {
			c.logger.Info("backend recovered", "url", b.URL.String())
		}
		b.SetState(backend.StateHealthy)
	} else {
		if b.IsAlive() {
			c.logger.Warn("backend became unhealthy", "url", b.URL.String(), "status", resp.StatusCode)
		}
		b.SetState(backend.StateUnhealthy)
	}
}
