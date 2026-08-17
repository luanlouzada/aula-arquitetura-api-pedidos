package exports

import (
	"errors"
	"sync"
	"testing"
)

// TestProvidedQueueRejectsImmediatelyWhenFull documenta a base pronta que o
// Handler usará depois da admissão pelo token bucket.
func TestProvidedQueueRejectsImmediatelyWhenFull(t *testing.T) {
	queue := NewQueue(1)
	if err := queue.TryEnqueue(Export{ID: "first"}); err != nil {
		t.Fatal(err)
	}
	if err := queue.TryEnqueue(Export{ID: "second"}); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("segunda entrada = %v; esperado ErrQueueFull", err)
	}
}

// TestProvidedQueueDrainsInFIFOOrder mostra que fechar o channel preserva os
// itens do buffer e encerra a leitura somente depois da drenagem.
func TestProvidedQueueDrainsInFIFOOrder(t *testing.T) {
	queue := NewQueue(2)
	_ = queue.TryEnqueue(Export{ID: "first"})
	_ = queue.TryEnqueue(Export{ID: "second"})
	queue.Close()
	for _, expected := range []string{"first", "second"} {
		item, open := <-queue.Exports()
		if !open || item.ID != expected {
			t.Fatalf("recebido id=%q open=%v; esperado %q aberto", item.ID, open, expected)
		}
	}
	if _, open := <-queue.Exports(); open {
		t.Fatal("channel deveria fechar depois de drenar o buffer")
	}
}

// TestProvidedQueueRejectsAfterClose documenta a proteção pronta usada quando
// um handler encontra o shutdown já em andamento.
func TestProvidedQueueRejectsAfterClose(t *testing.T) {
	queue := NewQueue(1)
	queue.Close()
	queue.Close()

	if err := queue.TryEnqueue(Export{ID: "late"}); !errors.Is(err, ErrQueueClosed) {
		t.Fatalf("tentativa após Close = %v; esperado ErrQueueClosed", err)
	}
}

// TestProvidedQueueCoordinatesConcurrentCloseAndEnqueue fixa um requisito da
// infraestrutura fornecida, não um dos TODOs de política deste módulo.
func TestProvidedQueueCoordinatesConcurrentCloseAndEnqueue(t *testing.T) {
	const producers = 16
	queue := NewQueue(1)
	start := make(chan struct{})
	results := make(chan error, producers)
	var workers sync.WaitGroup

	for index := 0; index < producers; index++ {
		workers.Add(1)
		go func(id int) {
			defer workers.Done()
			<-start
			results <- queue.TryEnqueue(Export{ID: string(rune('a' + id))})
		}(index)
	}

	close(start)
	queue.Close()
	workers.Wait()
	close(results)

	for err := range results {
		if err != nil && !errors.Is(err, ErrQueueFull) && !errors.Is(err, ErrQueueClosed) {
			t.Fatalf("resultado concorrente inesperado: %v", err)
		}
	}
}
