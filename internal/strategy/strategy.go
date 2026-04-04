package strategy

import (
	"github.com/anujdhakrey/load-balancer/internal/backend"
)

type Strategy interface {
	Name() string
	Next(backends []*backend.Backend) *backend.Backend
}
