# Load Balancer

A production-grade HTTP load balancer built from scratch in Go. Implements multiple load balancing strategies, active health checking, circuit breakers, per-IP rate limiting, and a runtime admin API — all with zero external dependencies beyond the standard library and a YAML parser.

## What This Project Does

This load balancer sits between clients and a pool of backend HTTP servers. It distributes incoming requests across healthy backends using configurable algorithms, automatically detects and routes around failed servers, and protects backends from overload.

```
                  ┌──────────────┐
                  │   Clients    │
                  └──────┬───────┘
                         │
                  ┌──────▼───────┐
                  │ Rate Limiter │  (token bucket, per-IP or global)
                  └──────┬───────┘
                         │
                  ┌──────▼───────┐
                  │  Load        │
                  │  Balancer    │  (round-robin / weighted / least-conn)
                  │  + Circuit   │
                  │    Breakers  │
                  └──┬───┬───┬──┘
                     │   │   │
              ┌──────▼┐ ┌▼──────┐ ┌▼──────┐
              │ BE #1 │ │ BE #2 │ │ BE #3 │
              └───────┘ └───────┘ └───────┘
                  ▲           ▲
                  └─────┬─────┘
              Health Checker (periodic)
```

### Core Features

| Feature | Description |
|---|---|
| **Round-Robin** | Evenly distributes requests across all healthy backends |
| **Weighted Round-Robin** | Routes traffic proportional to backend weights (e.g., send 3x traffic to a beefier server) |
| **Least Connections** | Sends each request to the backend with the fewest active connections |
| **Active Health Checks** | Periodically pings each backend's `/health` endpoint; automatically marks servers up/down |
| **Circuit Breaker** | Per-backend circuit breaker (closed → open → half-open) prevents cascading failures |
| **Rate Limiting** | Token bucket algorithm, configurable as global or per-IP |
| **Retry with Failover** | Automatically retries failed requests on different backends |
| **Admin API** | REST API for runtime backend management (add/remove/drain/stats) |
| **Graceful Shutdown** | Drains in-flight requests on SIGINT/SIGTERM before exiting |
| **Structured Logging** | JSON-formatted logs via `log/slog` |
| **Connection Draining** | Mark backends as "draining" — stops new traffic but doesn't kill existing connections |
| **Request Metrics** | Per-backend stats: active connections, total requests, failures, average latency |

## Project Structure

```
.
├── cmd/
│   ├── loadbalancer/          # Main entry point
│   │   └── main.go
│   └── example-backend/       # Simple test backend server
│       └── main.go
├── internal/
│   ├── admin/                 # Admin REST API server
│   ├── backend/               # Backend type with stats & state tracking
│   ├── circuitbreaker/        # Circuit breaker state machine
│   ├── config/                # YAML config loader with validation
│   ├── health/                # Active health checker (goroutine)
│   ├── pool/                  # Thread-safe backend pool management
│   ├── proxy/                 # Core load balancer + reverse proxy logic
│   ├── ratelimiter/           # Token bucket rate limiter
│   └── strategy/              # LB algorithm interface + implementations
├── config.yaml                # Default config (local development)
├── config.docker.yaml         # Docker Compose config
├── docker-compose.yaml
├── Dockerfile                 # Multi-stage build
├── Makefile
└── README.md
```

## How to Build and Run

### Prerequisites

- Go 1.22+
- (Optional) Docker & Docker Compose

### Option 1: Local (Makefile)

```bash
# Build everything
make build

# Start 3 example backend servers (ports 8081-8083)
make backends

# In another terminal, start the load balancer
make run
```

### Option 2: Manual

```bash
# Build
go build -o bin/loadbalancer ./cmd/loadbalancer
go build -o bin/example-backend ./cmd/example-backend

# Start backends
./bin/example-backend -port 8081 -name backend-1 &
./bin/example-backend -port 8082 -name backend-2 &
./bin/example-backend -port 8083 -name backend-3 &

# Start load balancer
./bin/loadbalancer -config config.yaml -log-level debug
```

### Option 3: Docker Compose

```bash
docker compose up --build
```

This starts 3 backend containers + the load balancer. The LB listens on `localhost:8080`, admin on `localhost:9090`.

### Test It

```bash
# Send requests through the load balancer
for i in $(seq 1 6); do curl -s http://localhost:8080/ | jq .server; done

# Expected output (round-robin):
# "backend-1"
# "backend-2"
# "backend-3"
# "backend-1"
# "backend-2"
# "backend-3"
```

### Admin API

```bash
# List all backends and their status
curl -s http://localhost:9090/backends | jq

# View detailed stats
curl -s http://localhost:9090/stats | jq

# Add a new backend at runtime
curl -X POST http://localhost:9090/backends \
  -H 'Content-Type: application/json' \
  -d '{"url": "http://localhost:8084", "weight": 2}'

# Drain a backend (stop sending new traffic)
curl -X POST http://localhost:9090/backends/drain \
  -H 'Content-Type: application/json' \
  -d '{"url": "http://localhost:8081"}'

# Remove a backend entirely
curl -X DELETE http://localhost:9090/backends \
  -H 'Content-Type: application/json' \
  -d '{"url": "http://localhost:8081"}'

# Health check (is the LB itself healthy?)
curl -s http://localhost:9090/health | jq
```

### Running Tests

```bash
make test

# With coverage
make test-cover
```

## Configuration

All configuration is in `config.yaml`:

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  read_timeout: 15s
  write_timeout: 15s
  idle_timeout: 60s
  max_retries: 3           # retries on different backends before giving up

admin:
  enabled: true
  host: "127.0.0.1"        # bind admin to loopback only
  port: 9090

health_check:
  interval: 10s             # how often to ping backends
  timeout: 5s               # per-check timeout
  path: "/health"           # endpoint to hit

# Options: round-robin, weighted-round-robin, least-connections
strategy: "round-robin"

rate_limit:
  enabled: true
  rate: 100                 # tokens per second
  burst_size: 200           # max burst
  per_ip: true              # per-IP buckets (false = single global bucket)
  cleanup_interval_sec: 300 # clean up stale IP buckets

backends:
  - url: "http://localhost:8081"
    weight: 1
  - url: "http://localhost:8082"
    weight: 1
  - url: "http://localhost:8083"
    weight: 1
```

## The Hard Parts

### 1. Circuit Breaker State Machine Correctness Under Concurrency

The circuit breaker has three states (closed → open → half-open → closed/open) and is accessed concurrently by every proxied request. The tricky part isn't implementing the state machine — it's getting the transitions right without races while keeping contention low.

The failure counter, success counter, and state must be atomically consistent: you can't have one goroutine reading "half-open" while another is transitioning it to "open" from a failure. A naive approach with separate atomic counters leads to subtle bugs where the state and counters get out of sync. The solution is a mutex guarding all state transitions together, but scoped tightly so it doesn't become a bottleneck on the hot path.

Another subtlety: when the circuit opens, requests already in flight shouldn't be retried on the same backend, but the circuit breaker can't know about the retry logic above it. The retry loop in the proxy has to check the breaker *before* dispatching, and the proxy's response recorder has to correctly propagate both transport errors (backend unreachable) and application errors (5xx responses) back to the breaker with the right semantics.

### 2. Retry-on-Different-Backend Without Infinite Loops

Retrying a failed request sounds simple until you consider: what if the same backend keeps getting selected? What if all backends fail? What if a backend fails during the proxy (half-written response)?

The retry logic tracks which backends have been attempted and tries to pick a different one on each retry. But the strategy (round-robin, least-conn) has its own selection logic — you can't easily tell it "skip this one". The solution is a two-layer approach: let the strategy pick, then check against the attempted set, and if all strategies return already-tried backends, we still retry (maybe a backend recovered between attempts). The max retry count is the hard limit that prevents infinite loops.

The half-written response problem is nasty. If the backend accepts the connection and starts sending headers but then dies mid-body, the response headers are already written to the client. At that point, you *cannot* retry — the HTTP response has started. The `responseRecorder` captures whether a proxy error occurred vs a full successful proxy to handle this correctly.

### 3. Coordination Between Health Checker, Pool, and Strategy

The health checker runs in its own goroutine, modifying backend state. The strategy reads backend state. The pool can be modified at runtime via the admin API (add/remove backends). All three are concurrent.

Getting the locking granularity right is important: the pool uses an `RWMutex` so multiple strategy calls can read concurrently, but add/remove takes a write lock. Each backend has its own mutex for health state, so the health checker doesn't need to lock the pool — it locks individual backends. This means a health check in progress doesn't block request routing.

But there's a subtle TOCTOU issue: the strategy gets a list of healthy backends, then picks one. Between those two operations, the health checker could mark that backend unhealthy. The round-robin handles this by double-checking `IsAlive()` in its selection loop rather than trusting the pre-filtered list.

### 4. Token Bucket Rate Limiter with Per-IP Tracking

A global rate limiter is trivial. Per-IP rate limiting means maintaining a map of token buckets that grows with every unique client. Without cleanup, this is a memory leak. The cleanup goroutine periodically evicts stale entries, but it has to lock the map to do so — and that lock contends with the hot path of every incoming request checking their bucket.

The design uses a two-level locking approach: the outer map has its own mutex (only held briefly to get/create a bucket), and each bucket has its own mutex for the token math. This means two different IPs never contend with each other, and the cleanup only briefly holds the map lock.

## Design Decisions

- **`internal/` packages**: Nothing is exported outside the module. This is a standalone binary, not a library.
- **`httputil.ReverseProxy`**: Uses the stdlib reverse proxy rather than raw TCP proxying. This handles HTTP correctly (hop-by-hop headers, request rewriting) without reimplementing the spec.
- **No external dependencies**: Only dependency is `gopkg.in/yaml.v3` for config parsing. Everything else is stdlib.
- **Structured logging (`log/slog`)**: JSON logs for production observability, with leveled output.
- **Strategy interface**: Adding a new algorithm (e.g., IP-hash, random) means implementing one method: `Next([]*Backend) *Backend`.
- **Admin API on separate port**: The admin API binds to a different port (default: loopback only) so it can be firewalled independently from the data plane.

## License

MIT
