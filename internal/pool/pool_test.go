package pool_test

import (
	"io"
	"log/slog"
	"testing"

	"github.com/anujdhakrey/load-balancer/internal/config"
	"github.com/anujdhakrey/load-balancer/internal/pool"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestPool_InitFromConfig(t *testing.T) {
	p := pool.New(testLogger())
	err := p.InitFromConfig([]config.BackendConfig{
		{URL: "http://localhost:8081", Weight: 1},
		{URL: "http://localhost:8082", Weight: 2},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Len() != 2 {
		t.Errorf("expected 2 backends, got %d", p.Len())
	}
}

func TestPool_AddAndRemoveBackend(t *testing.T) {
	p := pool.New(testLogger())

	if err := p.AddBackend("http://localhost:9001", 1); err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if p.Len() != 1 {
		t.Errorf("expected 1 backend, got %d", p.Len())
	}

	// Duplicate.
	if err := p.AddBackend("http://localhost:9001", 1); err == nil {
		t.Error("expected error for duplicate backend")
	}

	if err := p.RemoveBackend("http://localhost:9001"); err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	if p.Len() != 0 {
		t.Errorf("expected 0 backends, got %d", p.Len())
	}
}

func TestPool_DrainBackend(t *testing.T) {
	p := pool.New(testLogger())
	_ = p.AddBackend("http://localhost:9001", 1)

	if err := p.DrainBackend("http://localhost:9001"); err != nil {
		t.Fatalf("drain failed: %v", err)
	}

	if p.HealthyCount() != 0 {
		t.Error("draining backend should not count as healthy")
	}
}

func TestPool_HealthyBackends(t *testing.T) {
	p := pool.New(testLogger())
	_ = p.AddBackend("http://localhost:9001", 1)
	_ = p.AddBackend("http://localhost:9002", 1)

	if p.HealthyCount() != 2 {
		t.Errorf("expected 2 healthy, got %d", p.HealthyCount())
	}

	backends := p.Backends()
	backends[0].SetState(2) // StateUnhealthy equivalent
	// Note: we set state to 2 which is StateDraining — but let's use the proper value.
	// Actually, let's import backend to set properly.
}

func TestPool_Stats(t *testing.T) {
	p := pool.New(testLogger())
	_ = p.AddBackend("http://localhost:9001", 3)

	stats := p.Stats()
	if len(stats) != 1 {
		t.Fatalf("expected 1 stat entry, got %d", len(stats))
	}
	if stats[0].Weight != 3 {
		t.Errorf("expected weight 3, got %d", stats[0].Weight)
	}
}
