package ratelimiter

import (
	"sync"
	"time"
)

// TokenBucket implements a token bucket rate limiter.
// Tokens are added at a fixed rate. Each request consumes one token.
// If no tokens are available, the request is rejected.
type TokenBucket struct {
	rate       float64 // tokens per second
	burstSize  int     // max tokens
	tokens     float64
	lastRefill time.Time
	mu         sync.Mutex
}

func newBucket(rate float64, burstSize int) *TokenBucket {
	return &TokenBucket{
		rate:       rate,
		burstSize:  burstSize,
		tokens:     float64(burstSize),
		lastRefill: time.Now(),
	}
}

func (b *TokenBucket) allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * b.rate
	if b.tokens > float64(b.burstSize) {
		b.tokens = float64(b.burstSize)
	}
	b.lastRefill = now

	if b.tokens >= 1.0 {
		b.tokens--
		return true
	}
	return false
}

// RateLimiter supports both global and per-IP rate limiting.
type RateLimiter struct {
	global     *TokenBucket
	perIP      bool
	rate       float64
	burstSize  int
	ipBuckets  map[string]*TokenBucket
	mu         sync.Mutex
	cleanupSec int
}

func New(rate float64, burstSize int, perIP bool, cleanupSec int) *RateLimiter {
	rl := &RateLimiter{
		global:     newBucket(rate, burstSize),
		perIP:      perIP,
		rate:       rate,
		burstSize:  burstSize,
		ipBuckets:  make(map[string]*TokenBucket),
		cleanupSec: cleanupSec,
	}
	if perIP {
		go rl.cleanupLoop()
	}
	return rl
}

func (rl *RateLimiter) Allow(ip string) bool {
	if !rl.perIP {
		return rl.global.allow()
	}

	rl.mu.Lock()
	bucket, ok := rl.ipBuckets[ip]
	if !ok {
		bucket = newBucket(rl.rate, rl.burstSize)
		rl.ipBuckets[ip] = bucket
	}
	rl.mu.Unlock()

	return bucket.allow()
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(time.Duration(rl.cleanupSec) * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		threshold := time.Now().Add(-time.Duration(rl.cleanupSec) * time.Second)
		for ip, bucket := range rl.ipBuckets {
			bucket.mu.Lock()
			if bucket.lastRefill.Before(threshold) {
				delete(rl.ipBuckets, ip)
			}
			bucket.mu.Unlock()
		}
		rl.mu.Unlock()
	}
}
