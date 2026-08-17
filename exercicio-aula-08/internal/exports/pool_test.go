package exports

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"exercicio-aula-08/internal/telemetry"
)

// TestProvidedQueueAndPoolDrainAcceptedExports demonstra o pré-requisito já
// pronto: fechar a produção não descarta os quatro itens que estavam no buffer.
func TestProvidedQueueAndPoolDrainAcceptedExports(t *testing.T) {
	queue := NewQueue(4)
	metrics := telemetry.New("test")
	var processed atomic.Int64
	pool, err := NewPool(queue.Exports(), 2, func(context.Context, Export) error {
		processed.Add(1)
		return nil
	}, metrics)
	if err != nil {
		t.Fatal(err)
	}
	for range 4 {
		_ = queue.TryEnqueue(Export{ID: "export", EnqueuedAt: time.Now()})
	}
	queue.Close()
	pool.Start(context.Background())
	pool.Wait()
	if got := processed.Load(); got != 4 {
		t.Fatalf("exportações concluídas = %d; esperado 4", got)
	}
}

// TestProvidedPoolNeverExceedsWorkerLimit bloqueia dois Generators e confirma
// que a terceira Export permanece esperando até uma vaga ser liberada.
func TestProvidedPoolNeverExceedsWorkerLimit(t *testing.T) {
	queue := NewQueue(3)
	metrics := telemetry.New("test")
	entered := make(chan struct{}, 3)
	release := make(chan struct{})
	pool, err := NewPool(queue.Exports(), 2, func(context.Context, Export) error {
		entered <- struct{}{}
		<-release
		return nil
	}, metrics)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		_ = queue.TryEnqueue(Export{EnqueuedAt: time.Now()})
	}
	queue.Close()
	pool.Start(context.Background())
	for range 2 {
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("dois workers deveriam iniciar")
		}
	}
	select {
	case <-entered:
		t.Fatal("terceira Export iniciou antes de existir vaga de worker")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	pool.Wait()
}
