package jobs

import (
	"errors"
	"testing"
	"time"
)

// TestQueueRejectsWhenFull demonstra o backpressure sem iniciar workers. Com
// capacidade um, o primeiro Job ocupa o único espaço e a tentativa seguinte
// precisa falhar imediatamente com ErrQueueFull.
func TestQueueRejectsWhenFull(t *testing.T) {
	queue := NewQueue(1)
	job := Job{ID: "job-1", OrderID: 1, EnqueuedAt: time.Now()}
	if err := queue.TryEnqueue(job); err != nil {
		t.Fatalf("primeiro trabalho deveria ser aceito: %v", err)
	}
	if err := queue.TryEnqueue(job); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("segundo trabalho deveria receber ErrQueueFull, recebeu %v", err)
	}
	if got := queue.Depth(); got != 1 {
		t.Fatalf("profundidade = %d; esperado 1", got)
	}
}
