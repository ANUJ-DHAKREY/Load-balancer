package circuitbreaker_test

import (
	"testing"
	"time"

	"github.com/anujdhakrey/load-balancer/internal/circuitbreaker"
)

func TestCircuitBreaker_StartsClosedAndAllows(t *testing.T) {
	cb := circuitbreaker.New(3, 2, 1*time.Second)
	if !cb.Allow() {
		t.Error("expected closed circuit to allow requests")
	}
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cb := circuitbreaker.New(3, 2, 1*time.Second)

	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if cb.Allow() {
		t.Error("expected open circuit to reject requests")
	}
	if cb.State() != circuitbreaker.StateOpen {
		t.Errorf("expected state Open, got %v", cb.State())
	}
}

func TestCircuitBreaker_TransitionsToHalfOpenAfterTimeout(t *testing.T) {
	cb := circuitbreaker.New(3, 2, 50*time.Millisecond)

	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	time.Sleep(60 * time.Millisecond)

	if !cb.Allow() {
		t.Error("expected half-open circuit to allow a probe request")
	}
	if cb.State() != circuitbreaker.StateHalfOpen {
		t.Errorf("expected state HalfOpen, got %v", cb.State())
	}
}

func TestCircuitBreaker_ClosesAfterSuccessThreshold(t *testing.T) {
	cb := circuitbreaker.New(3, 2, 50*time.Millisecond)

	// Open the circuit.
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	// Wait for timeout.
	time.Sleep(60 * time.Millisecond)
	cb.Allow() // Transition to half-open.

	// Record enough successes to close.
	cb.RecordSuccess()
	cb.RecordSuccess()

	if cb.State() != circuitbreaker.StateClosed {
		t.Errorf("expected state Closed after recovery, got %v", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenFailureReopens(t *testing.T) {
	cb := circuitbreaker.New(3, 2, 50*time.Millisecond)

	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	time.Sleep(60 * time.Millisecond)
	cb.Allow() // half-open

	cb.RecordFailure() // should reopen

	if cb.State() != circuitbreaker.StateOpen {
		t.Errorf("expected state Open after half-open failure, got %v", cb.State())
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := circuitbreaker.New(3, 2, 1*time.Second)

	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	cb.Reset()

	if cb.State() != circuitbreaker.StateClosed {
		t.Errorf("expected Closed after reset, got %v", cb.State())
	}
	if !cb.Allow() {
		t.Error("expected reset circuit to allow requests")
	}
}
