package lifecycle

import (
	"context"
	"sync"
	"testing"
)

// TestShutdownUsesSafeOrder registra chamadas em vez de iniciar componentes
// reais e transforma a ordem de encerramento em contrato executável.
func TestShutdownUsesSafeOrder(t *testing.T) {
	var events []string
	record := func(event string) { events = append(events, event) }
	steps := Steps{
		MarkNotReady:     func() { record("not-ready") },
		ShutdownHTTP:     func(context.Context) error { record("http"); return nil },
		ForceCloseHTTP:   func() error { record("force-http"); return nil },
		CloseQueue:       func() { record("queue") },
		WaitWorkers:      func() { record("workers") },
		ForceStopWorkers: func() { record("force-workers") },
	}
	if err := Shutdown(context.Background(), 0, steps); err != nil {
		t.Fatal(err)
	}
	want := []string{"not-ready", "http", "queue", "workers"}
	if len(events) != len(want) {
		t.Fatalf("eventos = %v; esperado %v", events, want)
	}
	for index := range want {
		if events[index] != want[index] {
			t.Fatalf("eventos = %v; esperado %v", events, want)
		}
	}
}

// TestShutdownForcesWorkersAfterDeadline simula workers presos até receberem o
// cancelamento forçado exigido pelo prazo.
func TestShutdownForcesWorkersAfterDeadline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var mutex sync.Mutex
	forcedHTTP := false
	forcedWorkers := false
	releaseWorkers := make(chan struct{})
	steps := Steps{
		MarkNotReady: func() {},
		ShutdownHTTP: func(context.Context) error { return context.Canceled },
		ForceCloseHTTP: func() error {
			mutex.Lock()
			forcedHTTP = true
			mutex.Unlock()
			return nil
		},
		CloseQueue:  func() {},
		WaitWorkers: func() { <-releaseWorkers },
		ForceStopWorkers: func() {
			mutex.Lock()
			forcedWorkers = true
			mutex.Unlock()
			close(releaseWorkers)
		},
	}
	if err := Shutdown(ctx, 0, steps); err == nil {
		t.Fatal("prazo encerrado deveria produzir erro")
	}
	mutex.Lock()
	defer mutex.Unlock()
	if !forcedHTTP {
		t.Fatal("HTTP deveria ser fechado à força depois de ShutdownHTTP falhar")
	}
	if !forcedWorkers {
		t.Fatal("workers deveriam ser cancelados após o prazo")
	}
}

// TestShutdownRejectsMissingStep exige validação antes de uma transição parcial.
func TestShutdownRejectsMissingStep(t *testing.T) {
	if err := Shutdown(context.Background(), 0, Steps{}); err == nil {
		t.Fatal("etapas ausentes deveriam ser rejeitadas")
	}
}
