package admission

import (
	"math"
	"testing"
	"time"
)

// TestTokenBucketAllowsBurstThenRejects congela o tempo e verifica a capacidade
// inicial do recipiente sem depender da velocidade da máquina.
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

// TestTokenBucketRefillsOverTimeAndCapsAtBurst separa rate de burst ao avançar
// manualmente 500ms e depois um intervalo longo.
func TestTokenBucketRefillsOverTimeAndCapsAtBurst(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	bucket, err := newTokenBucket(2, 3, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		_ = bucket.Allow()
	}
	now = now.Add(500 * time.Millisecond)
	if !bucket.Allow() {
		t.Fatal("taxa 2/s deveria repor uma permissão após 500ms")
	}
	if bucket.Allow() {
		t.Fatal("a segunda tentativa deveria ser rejeitada")
	}
	now = now.Add(10 * time.Second)
	allowed := 0
	for range 4 {
		if bucket.Allow() {
			allowed++
		}
	}
	if allowed != 3 {
		t.Fatalf("permissões acumuladas = %d; esperado burst máximo 3", allowed)
	}
}

// TestTokenBucketRejectsInvalidConfiguration define os erros de montagem que o
// construtor precisa devolver antes de criar o servidor.
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
