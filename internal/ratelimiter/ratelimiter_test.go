package ratelimiter_test

import (
	"testing"

	"github.com/anujdhakrey/load-balancer/internal/ratelimiter"
)

func TestRateLimiter_GlobalAllowsBurst(t *testing.T) {
	rl := ratelimiter.New(10, 5, false, 300)

	// Should allow up to burst size.
	for i := 0; i < 5; i++ {
		if !rl.Allow("") {
			t.Errorf("request %d should be allowed within burst", i+1)
		}
	}

	// 6th should be rejected.
	if rl.Allow("") {
		t.Error("expected rejection after burst exhausted")
	}
}

func TestRateLimiter_PerIP(t *testing.T) {
	rl := ratelimiter.New(10, 3, true, 300)

	// Each IP gets its own bucket.
	for i := 0; i < 3; i++ {
		if !rl.Allow("1.2.3.4") {
			t.Error("ip1 should be allowed")
		}
	}
	if rl.Allow("1.2.3.4") {
		t.Error("ip1 should be rate limited")
	}

	// Different IP should still have tokens.
	if !rl.Allow("5.6.7.8") {
		t.Error("ip2 should be allowed")
	}
}
