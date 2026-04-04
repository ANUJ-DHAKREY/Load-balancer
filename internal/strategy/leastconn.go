package strategy

import (
	"github.com/anujdhakrey/load-balancer/internal/backend"
)

type LeastConnections struct{}

func NewLeastConnections() *LeastConnections {
	return &LeastConnections{}
}

func (l *LeastConnections) Name() string {
	return "least-connections"
}

func (l *LeastConnections) Next(backends []*backend.Backend) *backend.Backend {
	if len(backends) == 0 {
		return nil
	}

	var best *backend.Backend
	minConns := int64(1<<63 - 1)

	for _, b := range backends {
		if !b.IsAlive() {
			continue
		}
		conns := b.ActiveConnections()
		if conns < minConns {
			minConns = conns
			best = b
		}
	}

	return best
}
