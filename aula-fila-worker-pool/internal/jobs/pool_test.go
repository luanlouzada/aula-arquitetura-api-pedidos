package jobs

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"aula-fila-worker-pool/internal/telemetry"
)

// TestPoolDrainsQueueAndBuildsBatches substitui o processamento real por uma
// função que registra os tamanhos recebidos. Depois de fechar a fila, o único
// worker deve drenar cinco Jobs em um lote cheio de três e um lote parcial de
// dois, sem perder nenhum item.
func TestPoolDrainsQueueAndBuildsBatches(t *testing.T) {
	queue := NewQueue(10)
	metrics := &telemetry.Metrics{}
	var mutex sync.Mutex
	var batchSizes []int
	processor := func(_ context.Context, batch []Job) error {
		mutex.Lock()
		batchSizes = append(batchSizes, len(batch))
		mutex.Unlock()
		return nil
	}
	pool, err := NewPool(
		queue.Jobs(),
		1,
		3,
		10*time.Millisecond,
		processor,
		metrics,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatal(err)
	}
	for id := 1; id <= 5; id++ {
		_ = queue.TryEnqueue(Job{ID: "job", OrderID: int64(id), EnqueuedAt: time.Now()})
		metrics.RecordAccepted()
	}
	queue.Close()
	pool.Start(context.Background())
	pool.Wait()

	snapshot := metrics.Snapshot(queue.Depth(), queue.Capacity(), 1)
	if snapshot.Jobs.Completed != 5 {
		t.Fatalf("concluídos = %d; esperado 5", snapshot.Jobs.Completed)
	}
	mutex.Lock()
	defer mutex.Unlock()
	if len(batchSizes) != 2 || batchSizes[0] != 3 || batchSizes[1] != 2 {
		t.Fatalf("lotes = %v; esperado [3 2]", batchSizes)
	}
}
