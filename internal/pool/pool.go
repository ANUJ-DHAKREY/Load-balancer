package pool

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/anujdhakrey/load-balancer/internal/backend"
	"github.com/anujdhakrey/load-balancer/internal/config"
)

type Pool struct {
	backends []*backend.Backend
	mu       sync.RWMutex
	logger   *slog.Logger
}

func New(logger *slog.Logger) *Pool {
	return &Pool{
		logger: logger,
	}
}

func (p *Pool) InitFromConfig(configs []config.BackendConfig) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, cfg := range configs {
		b, err := backend.New(cfg.URL, cfg.Weight)
		if err != nil {
			return fmt.Errorf("creating backend %q: %w", cfg.URL, err)
		}
		p.backends = append(p.backends, b)
		p.logger.Info("registered backend", "url", cfg.URL, "weight", cfg.Weight)
	}
	return nil
}

func (p *Pool) AddBackend(rawURL string, weight int) error {
	if weight < 1 {
		weight = 1
	}
	b, err := backend.New(rawURL, weight)
	if err != nil {
		return fmt.Errorf("creating backend %q: %w", rawURL, err)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for _, existing := range p.backends {
		if existing.URL.String() == b.URL.String() {
			return fmt.Errorf("backend %q already exists", rawURL)
		}
	}

	p.backends = append(p.backends, b)
	p.logger.Info("added backend", "url", rawURL, "weight", weight)
	return nil
}

func (p *Pool) RemoveBackend(rawURL string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i, b := range p.backends {
		if b.URL.String() == rawURL {
			p.backends = append(p.backends[:i], p.backends[i+1:]...)
			p.logger.Info("removed backend", "url", rawURL)
			return nil
		}
	}
	return fmt.Errorf("backend %q not found", rawURL)
}

func (p *Pool) DrainBackend(rawURL string) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, b := range p.backends {
		if b.URL.String() == rawURL {
			b.SetState(backend.StateDraining)
			p.logger.Info("draining backend", "url", rawURL)
			return nil
		}
	}
	return fmt.Errorf("backend %q not found", rawURL)
}

func (p *Pool) Backends() []*backend.Backend {
	p.mu.RLock()
	defer p.mu.RUnlock()
	result := make([]*backend.Backend, len(p.backends))
	copy(result, p.backends)
	return result
}

func (p *Pool) HealthyBackends() []*backend.Backend {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var healthy []*backend.Backend
	for _, b := range p.backends {
		if b.IsAlive() {
			healthy = append(healthy, b)
		}
	}
	return healthy
}

func (p *Pool) Len() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.backends)
}

func (p *Pool) HealthyCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	count := 0
	for _, b := range p.backends {
		if b.IsAlive() {
			count++
		}
	}
	return count
}

func (p *Pool) Stats() []backend.Stats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := make([]backend.Stats, len(p.backends))
	for i, b := range p.backends {
		stats[i] = b.Stats()
	}
	return stats
}
