package telemetry

import (
	"testing"
	"time"
)

// TestSnapshotSeparatesAcceptanceFromUsefulCompletion garante que 202 e
// goodput nunca sejam apresentados como o mesmo evento.
func TestSnapshotSeparatesAcceptanceFromUsefulCompletion(t *testing.T) {
	metrics := New("api-test")
	metrics.RecordRequest()
	metrics.RecordAccepted()
	metrics.RecordJobStarted(10 * time.Millisecond)
	metrics.RecordJobFinished(nil)
	snapshot := metrics.Snapshot(Runtime{
		Ready:         true,
		QueueDepth:    1,
		QueueCapacity: 4,
		Workers:       2,
		RatePerSecond: 8,
		Burst:         4,
		Tokens:        2.5,
	})

	if snapshot.Requests.Accepted202 != 1 || snapshot.Jobs.Completed != 1 {
		t.Fatalf("snapshot inesperado: %+v", snapshot)
	}
	if snapshot.Queue.Utilization != 0.25 {
		t.Fatalf("utilização = %v; esperado 0.25", snapshot.Queue.Utilization)
	}
	if snapshot.Jobs.AverageQueueWaitMS != 10 {
		t.Fatalf("espera média = %vms; esperado 10ms", snapshot.Jobs.AverageQueueWaitMS)
	}
	if snapshot.Jobs.GoodputJobsPerSecond <= 0 {
		t.Fatal("uma conclusão deveria produzir goodput positivo")
	}
}
