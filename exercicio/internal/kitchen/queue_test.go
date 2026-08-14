package kitchen

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestNewQueueCreatesEmptyBufferWithCapacity(t *testing.T) {
	queue := NewQueue(3)

	if got := queue.Depth(); got != 0 {
		t.Fatalf("profundidade inicial = %d; esperado 0", got)
	}
	if got := queue.Capacity(); got != 3 {
		t.Fatalf("capacidade = %d; esperado 3", got)
	}
}

func TestNewQueueRejectsInvalidCapacity(t *testing.T) {
	for _, capacity := range []int{0, -1} {
		t.Run(fmt.Sprintf("capacity=%d", capacity), func(t *testing.T) {
			// recover permite ao teste observar o panic exigido pelo contrato sem
			// encerrar todo o processo de testes.
			defer func() {
				if recover() == nil {
					t.Fatalf("NewQueue(%d) deveria causar panic", capacity)
				}
			}()
			_ = NewQueue(capacity)
		})
	}
}

func TestQueueTryEnqueueAcceptsUntilFull(t *testing.T) {
	queue := NewQueue(2)
	for _, id := range []string{"ticket-1", "ticket-2"} {
		err := tryEnqueueWithin(t, queue, Ticket{ID: id, Dish: "lasanha", EnqueuedAt: time.Now()})
		if err != nil {
			t.Fatalf("%s deveria ser aceito: %v", id, err)
		}
	}

	err := tryEnqueueWithin(t, queue, Ticket{ID: "ticket-3", Dish: "pizza", EnqueuedAt: time.Now()})
	if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("terceira comanda deveria receber ErrQueueFull, recebeu %v", err)
	}
	if got := queue.Depth(); got != 2 {
		t.Fatalf("profundidade = %d; esperado 2", got)
	}
}

func tryEnqueueWithin(t *testing.T, queue *Queue, ticket Ticket) error {
	t.Helper()
	// A chamada fica em outra goroutine para que uma implementação bloqueante
	// produza uma falha clara em vez de deixar o teste parado indefinidamente.
	result := make(chan error, 1)
	go func() {
		result <- queue.TryEnqueue(ticket)
	}()
	select {
	case err := <-result:
		return err
	case <-time.After(250 * time.Millisecond):
		t.Fatal("TryEnqueue bloqueou; a decisão de aceitar ou recusar deve ser imediata")
		return nil
	}
}

func TestQueueCloseDrainsAcceptedTicketsAndClosesChannel(t *testing.T) {
	queue := NewQueue(2)
	for _, id := range []string{"ticket-1", "ticket-2"} {
		if err := queue.TryEnqueue(Ticket{ID: id, Dish: "pizza", EnqueuedAt: time.Now()}); err != nil {
			t.Fatalf("enfileirar %s: %v", id, err)
		}
	}
	queue.Close()

	tickets := queue.Tickets()
	for _, expectedID := range []string{"ticket-1", "ticket-2"} {
		select {
		case ticket, ok := <-tickets:
			if !ok {
				t.Fatalf("fila fechou antes de entregar %s", expectedID)
			}
			if ticket.ID != expectedID {
				t.Fatalf("ticket recebido = %q; esperado %q", ticket.ID, expectedID)
			}
		case <-time.After(time.Second):
			t.Fatalf("tempo excedido esperando %s", expectedID)
		}
	}

	select {
	case _, ok := <-tickets:
		if ok {
			t.Fatal("channel deveria estar fechado depois de drenar os tickets")
		}
	case <-time.After(time.Second):
		t.Fatal("channel não sinalizou fechamento")
	}
}
