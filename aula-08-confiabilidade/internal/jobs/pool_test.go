package jobs

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"aula-08-confiabilidade/internal/telemetry"
)

// TestPoolDrainsAcceptedJobsAfterQueueCloses fecha a produção e verifica que os
// quatro valores já aceitos chegam ao Processor antes de Wait retornar.
func TestPoolDrainsAcceptedJobsAfterQueueCloses(t *testing.T) {
	queue := NewQueue(4)
	metrics := telemetry.New("test")
	var processed atomic.Int64
	pool, err := NewPool(queue.Jobs(), 2, func(context.Context, Job) error {
		processed.Add(1)
		return nil
	}, metrics)
	if err != nil {
		t.Fatal(err)
	}
	for id := 1; id <= 4; id++ {
		_ = queue.TryEnqueue(Job{ID: "job", EnqueuedAt: time.Now()})
	}
	queue.Close()
	pool.Start(context.Background())
	pool.Wait()
	if got := processed.Load(); got != 4 {
		t.Fatalf("processados = %d; esperado 4", got)
	}
}

// TestPoolNeverRunsMoreThanConfiguredWorkers bloqueia dois Processors e prova
// que o terceiro Job só começa depois que uma dessas vagas é liberada.
func TestPoolNeverRunsMoreThanConfiguredWorkers(t *testing.T) {
	queue := NewQueue(3)
	metrics := telemetry.New("test")
	entered := make(chan struct{}, 3)
	release := make(chan struct{})
	pool, err := NewPool(queue.Jobs(), 2, func(context.Context, Job) error {
		entered <- struct{}{}
		<-release
		return nil
	}, metrics)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		_ = queue.TryEnqueue(Job{EnqueuedAt: time.Now()})
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
		t.Fatal("terceiro Job iniciou antes de existir uma vaga de worker")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	pool.Wait()
}
