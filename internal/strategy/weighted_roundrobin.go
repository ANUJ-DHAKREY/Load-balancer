package strategy

import (
	"sync"
	"sync/atomic"

	"github.com/anujdhakrey/load-balancer/internal/backend"
)

type WeightedRoundRobin struct {
	mu      sync.Mutex
	counter uint64
}

func NewWeightedRoundRobin() *WeightedRoundRobin {
	return &WeightedRoundRobin{}
}

func (w *WeightedRoundRobin) Name() string {
	return "weighted-round-robin"
}

// Next implements smooth weighted round-robin (SWRR).
// Each backend gets traffic proportional to its weight.
// Algorithm:
//  1. Build expanded list where each backend appears `weight` times
//  2. Round-robin through the expanded list
//
// This is a simple approach. For very large weight values, consider
// the Nginx smooth weighted round-robin algorithm instead.
func (w *WeightedRoundRobin) Next(backends []*backend.Backend) *backend.Backend {
	if len(backends) == 0 {
		return nil
	}

	// Build the weighted list of alive backends.
	var expanded []*backend.Backend
	for _, b := range backends {
		if !b.IsAlive() {
			continue
		}
		for j := 0; j < b.Weight; j++ {
			expanded = append(expanded, b)
		}
	}

	if len(expanded) == 0 {
		return nil
	}

	n := uint64(len(expanded))
	idx := atomic.AddUint64(&w.counter, 1) - 1
	return expanded[idx%n]
}
