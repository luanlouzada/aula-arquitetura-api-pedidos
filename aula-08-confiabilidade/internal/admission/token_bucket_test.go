package admission

import (
	"math"
	"testing"
	"time"
)

// TestTokenBucketAllowsBurstThenRejects congela o relógio para provar que burst
// permite somente três admissões no mesmo instante.
func TestTokenBucketAllowsBurstThenRejects(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	bucket, err := newTokenBucket(2, 3, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}

	for attempt := 1; attempt <= 3; attempt++ {
		if !bucket.Allow() {
			t.Fatalf("tentativa %d deveria usar uma das três permissões iniciais", attempt)
		}
	}
	if bucket.Allow() {
		t.Fatal("quarta tentativa imediata deveria ser rejeitada")
	}
}

// TestTokenBucketRefillsAtConfiguredRateWithoutExceedingBurst avança o relógio
// manualmente e separa velocidade de reposição de capacidade do recipiente.
func TestTokenBucketRefillsAtConfiguredRateWithoutExceedingBurst(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	bucket, err := newTokenBucket(2, 3, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		_ = bucket.Allow()
	}

	now = now.Add(500 * time.Millisecond) // taxa 2/s repõe exatamente uma permissão.
	if !bucket.Allow() {
		t.Fatal("uma permissão deveria ter sido reposta após 500ms")
	}
	if bucket.Allow() {
		t.Fatal("somente uma permissão deveria estar disponível")
	}

	now = now.Add(10 * time.Second)
	allowed := 0
	for range 4 {
		if bucket.Allow() {
			allowed++
		}
	}
	if allowed != 3 {
		t.Fatalf("permissões após longa espera = %d; esperado burst máximo 3", allowed)
	}
}

// TestTokenBucketRejectsInvalidConfiguration garante que a aplicação falhe na
// inicialização em vez de executar um limitador que nunca admite operações.
func TestTokenBucketRejectsInvalidConfiguration(t *testing.T) {
	for _, test := range []struct {
		rate  float64
		burst int
	}{
		{rate: 0, burst: 1},
		{rate: -1, burst: 1},
		{rate: math.NaN(), burst: 1},
		{rate: math.Inf(1), burst: 1},
		{rate: 1, burst: 0},
	} {
		if _, err := NewTokenBucket(test.rate, test.burst); err == nil {
			t.Fatalf("rate=%v burst=%d deveria falhar", test.rate, test.burst)
		}
	}
}
