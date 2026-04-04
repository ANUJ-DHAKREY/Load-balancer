package strategy

import (
	"sync/atomic"

	"github.com/anujdhakrey/load-balancer/internal/backend"
)

type RoundRobin struct {
	counter uint64
}

func NewRoundRobin() *RoundRobin {
	return &RoundRobin{}
}

func (r *RoundRobin) Name() string {
	return "round-robin"
}

func (r *RoundRobin) Next(backends []*backend.Backend) *backend.Backend {
	if len(backends) == 0 {
		return nil
	}

	n := uint64(len(backends))
	idx := atomic.AddUint64(&r.counter, 1) - 1

	// Try each backend starting from the calculated index.
	// This handles the case where some backends become unhealthy
	// between getting the healthy list and selecting one.
	for i := uint64(0); i < n; i++ {
		b := backends[(idx+i)%n]
		if b.IsAlive() {
			return b
		}
	}
	return nil
}
