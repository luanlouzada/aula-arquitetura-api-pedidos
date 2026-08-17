package telemetry

import (
	"testing"
	"time"
)

// TestProvidedSnapshotSeparates202FromCompletion documenta as métricas prontas
// que os TODOs do Handler e do Pool alimentam.
func TestProvidedSnapshotSeparates202FromCompletion(t *testing.T) {
	metrics := New("exporter-test")
	metrics.RecordRequest()
	metrics.RecordAccepted()
	metrics.RecordExportStarted(10 * time.Millisecond)
	metrics.RecordExportFinished(nil)
	snapshot := metrics.Snapshot(Runtime{
		Ready:         true,
		QueueDepth:    1,
		QueueCapacity: 4,
		Workers:       2,
		RatePerSecond: 8,
		Burst:         4,
		Tokens:        2.5,
	})

	if snapshot.Requests.Accepted202 != 1 || snapshot.Exports.Completed != 1 {
		t.Fatalf("snapshot inesperado: %+v", snapshot)
	}
	if snapshot.Queue.Utilization != 0.25 {
		t.Fatalf("utilização = %v; esperado 0.25", snapshot.Queue.Utilization)
	}
	if snapshot.Exports.AverageQueueWaitMS != 10 {
		t.Fatalf("espera média = %vms; esperado 10ms", snapshot.Exports.AverageQueueWaitMS)
	}
	if snapshot.Exports.GoodputPerSecond <= 0 {
		t.Fatal("uma conclusão deveria produzir goodput positivo")
	}
}
