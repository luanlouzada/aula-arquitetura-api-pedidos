package jobs

import (
	"errors"
	"sync"
	"testing"
)

// TestQueueRejectsImmediatelyWhenFull preenche o único espaço e confirma que a
// próxima entrada recebe o erro de capacidade esperado.
func TestQueueRejectsImmediatelyWhenFull(t *testing.T) {
	queue := NewQueue(1)
	if err := queue.TryEnqueue(Job{ID: "one"}); err != nil {
		t.Fatal(err)
	}
	if err := queue.TryEnqueue(Job{ID: "two"}); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("segunda tentativa = %v; esperado ErrQueueFull", err)
	}
}

// TestQueueCloseKeepsAcceptedJobsInFIFOOrder mostra que Close encerra produção,
// mas não apaga o buffer, e que um único consumidor recebe na ordem de entrada.
func TestQueueCloseKeepsAcceptedJobsInFIFOOrder(t *testing.T) {
	queue := NewQueue(2)
	_ = queue.TryEnqueue(Job{ID: "first"})
	_ = queue.TryEnqueue(Job{ID: "second"})
	queue.Close()

	for _, expected := range []string{"first", "second"} {
		job, open := <-queue.Jobs()
		if !open || job.ID != expected {
			t.Fatalf("recebido id=%q open=%v; esperado %q aberto", job.ID, open, expected)
		}
	}
	if _, open := <-queue.Jobs(); open {
		t.Fatal("channel deveria sinalizar fechamento depois de drenar o buffer")
	}
}

// TestQueueRejectsAfterCloseAndCloseIsIdempotent cobre o estado operacional
// encontrado por um handler atrasado durante o encerramento forçado.
func TestQueueRejectsAfterCloseAndCloseIsIdempotent(t *testing.T) {
	queue := NewQueue(1)
	queue.Close()
	queue.Close()

	if err := queue.TryEnqueue(Job{ID: "late"}); !errors.Is(err, ErrQueueClosed) {
		t.Fatalf("tentativa após Close = %v; esperado ErrQueueClosed", err)
	}
}

// TestQueueCoordinatesConcurrentCloseAndEnqueue exercita a sincronização que
// impede um send concorrente de atingir um channel já fechado.
func TestQueueCoordinatesConcurrentCloseAndEnqueue(t *testing.T) {
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
			results <- queue.TryEnqueue(Job{ID: string(rune('a' + id))})
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
