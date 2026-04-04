package strategy_test

import (
	"testing"

	"github.com/anujdhakrey/load-balancer/internal/backend"
	"github.com/anujdhakrey/load-balancer/internal/strategy"
)

func makeBackends(t *testing.T, urls ...string) []*backend.Backend {
	t.Helper()
	var backends []*backend.Backend
	for _, u := range urls {
		b, err := backend.New(u, 1)
		if err != nil {
			t.Fatalf("failed to create backend: %v", err)
		}
		backends = append(backends, b)
	}
	return backends
}

func TestRoundRobin_DistributesEvenly(t *testing.T) {
	backends := makeBackends(t, "http://a:80", "http://b:80", "http://c:80")
	rr := strategy.NewRoundRobin()

	counts := make(map[string]int)
	for i := 0; i < 300; i++ {
		b := rr.Next(backends)
		if b == nil {
			t.Fatal("expected a backend, got nil")
		}
		counts[b.URL.String()]++
	}

	for _, url := range []string{"http://a:80", "http://b:80", "http://c:80"} {
		if counts[url] != 100 {
			t.Errorf("expected 100 requests to %s, got %d", url, counts[url])
		}
	}
}

func TestRoundRobin_EmptyBackends(t *testing.T) {
	rr := strategy.NewRoundRobin()
	if b := rr.Next(nil); b != nil {
		t.Error("expected nil for empty backends")
	}
}

func TestRoundRobin_SkipsUnhealthy(t *testing.T) {
	backends := makeBackends(t, "http://a:80", "http://b:80")
	backends[0].SetState(backend.StateUnhealthy)

	rr := strategy.NewRoundRobin()
	for i := 0; i < 10; i++ {
		b := rr.Next(backends)
		if b == nil {
			t.Fatal("expected a backend")
		}
		if b.URL.String() == "http://a:80" {
			t.Error("got unhealthy backend")
		}
	}
}

func TestLeastConnections_SelectsLeast(t *testing.T) {
	backends := makeBackends(t, "http://a:80", "http://b:80", "http://c:80")
	// a has 5 conns, b has 2, c has 0
	for i := 0; i < 5; i++ {
		backends[0].IncrementConnections()
	}
	for i := 0; i < 2; i++ {
		backends[1].IncrementConnections()
	}

	lc := strategy.NewLeastConnections()
	b := lc.Next(backends)
	if b == nil {
		t.Fatal("expected a backend")
	}
	if b.URL.String() != "http://c:80" {
		t.Errorf("expected backend c (0 conns), got %s", b.URL.String())
	}
}

func TestLeastConnections_SkipsUnhealthy(t *testing.T) {
	backends := makeBackends(t, "http://a:80", "http://b:80")
	backends[1].SetState(backend.StateUnhealthy)
	// a has 10 conns, b has 0 but is unhealthy
	for i := 0; i < 10; i++ {
		backends[0].IncrementConnections()
	}

	lc := strategy.NewLeastConnections()
	b := lc.Next(backends)
	if b == nil {
		t.Fatal("expected a backend")
	}
	if b.URL.String() != "http://a:80" {
		t.Errorf("expected backend a (only healthy), got %s", b.URL.String())
	}
}

func TestWeightedRoundRobin_RespectsWeights(t *testing.T) {
	b1, _ := backend.New("http://a:80", 3)
	b2, _ := backend.New("http://b:80", 1)
	backends := []*backend.Backend{b1, b2}

	wrr := strategy.NewWeightedRoundRobin()

	counts := make(map[string]int)
	for i := 0; i < 400; i++ {
		b := wrr.Next(backends)
		if b == nil {
			t.Fatal("expected a backend")
		}
		counts[b.URL.String()]++
	}

	// With weights 3:1, expect ~75%/25% split.
	aCount := counts["http://a:80"]
	bCount := counts["http://b:80"]
	if aCount != 300 || bCount != 100 {
		t.Errorf("expected 300:100 split, got %d:%d", aCount, bCount)
	}
}
