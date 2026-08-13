package main

import (
	"testing"
	"time"
)

func TestPercentileUsesOrderedObservation(t *testing.T) {
	values := []time.Duration{5 * time.Millisecond, time.Millisecond, 4 * time.Millisecond, 2 * time.Millisecond, 3 * time.Millisecond}
	if got, want := percentile(values, 0.50), 3*time.Millisecond; got != want {
		t.Fatalf("p50 = %s, want %s", got, want)
	}
	if got, want := percentile(values, 0.95), 5*time.Millisecond; got != want {
		t.Fatalf("p95 = %s, want %s", got, want)
	}
}
