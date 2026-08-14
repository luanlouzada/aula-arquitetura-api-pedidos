package kitchen

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestPoolDrainsQueue(t *testing.T) {
	queue := NewQueue(10)
	var mutex sync.Mutex
	var received []string
	// Este Processor não cozinha: ele apenas registra cada ticket que o Pool
	// entregou. O mutex protege a slice porque dois workers podem chamá-lo juntos.
	processor := func(_ context.Context, ticket Ticket) error {
		mutex.Lock()
		received = append(received, ticket.ID)
		mutex.Unlock()
		return nil
	}
	pool, err := NewPool(queue.Tickets(), 2, processor)
	if err != nil {
		t.Fatal(err)
	}

	for i := 1; i <= 5; i++ {
		ticket := Ticket{
			ID:         fmt.Sprintf("ticket-%d", i),
			Dish:       "pizza",
			EnqueuedAt: time.Now(),
		}
		if err := queue.TryEnqueue(ticket); err != nil {
			t.Fatalf("enfileirar %s: %v", ticket.ID, err)
		}
	}
	queue.Close()
	startPool(t, pool, context.Background())
	waitForPool(t, pool)

	mutex.Lock()
	defer mutex.Unlock()
	if len(received) != 5 {
		t.Fatalf("recebidos = %d; esperado 5 (%v)", len(received), received)
	}
}

func TestPoolRespectsWorkerLimit(t *testing.T) {
	queue := NewQueue(3)
	for i := 1; i <= 3; i++ {
		if err := queue.TryEnqueue(Ticket{ID: fmt.Sprintf("ticket-%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	queue.Close()

	started := make(chan struct{}, 3)
	release := make(chan struct{})
	var stateMutex sync.Mutex
	active := 0
	peak := 0
	processed := 0
	processor := func(_ context.Context, _ Ticket) error {
		stateMutex.Lock()
		active++
		if active > peak {
			peak = active
		}
		stateMutex.Unlock()

		// O teste mantém os dois primeiros cozinheiros ocupados. Se Start criar
		// concorrência além do limite, o terceiro ticket também chegará a started.
		started <- struct{}{}
		<-release

		stateMutex.Lock()
		active--
		processed++
		stateMutex.Unlock()
		return nil
	}

	pool, err := NewPool(queue.Tickets(), 2, processor)
	if err != nil {
		t.Fatal(err)
	}
	startPool(t, pool, context.Background())

	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("dois cozinheiros não começaram a trabalhar")
		}
	}
	select {
	case <-started:
		t.Fatal("terceiro ticket começou enquanto os dois cozinheiros estavam ocupados")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	waitForPool(t, pool)
	stateMutex.Lock()
	defer stateMutex.Unlock()
	if peak != 2 {
		t.Fatalf("pico de processamento simultâneo = %d; esperado 2", peak)
	}
	if processed != 3 {
		t.Fatalf("tickets processados = %d; esperado 3", processed)
	}
}

func TestPoolStopsWhenContextIsCanceled(t *testing.T) {
	// O channel permanece aberto e vazio. Portanto, somente o cancelamento pode
	// acordar os workers e permitir que Wait termine.
	tickets := make(chan Ticket)
	processor := func(context.Context, Ticket) error { return nil }
	pool, err := NewPool(tickets, 2, processor)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	startPool(t, pool, ctx)
	cancel()
	waitForPool(t, pool)
}

func startPool(t *testing.T, pool *Pool, ctx context.Context) {
	t.Helper()
	// Start deve apenas criar os workers e retornar. Rodá-lo em outra goroutine
	// permite detectar uma implementação que chamou Wait dentro de Start.
	returned := make(chan struct{})
	go func() {
		pool.Start(ctx)
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("Start bloqueou; ele deve iniciar os workers e retornar sem chamar Wait")
	}
}

func waitForPool(t *testing.T, pool *Pool) {
	t.Helper()
	// Wait é bloqueante. Executá-lo em outra goroutine permite impor um prazo e
	// explicar que o Pool não encerrou, em vez de travar a suíte de testes.
	done := make(chan struct{})
	go func() {
		pool.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("o pool não terminou dentro do prazo")
	}
}
