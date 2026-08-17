package lifecycle

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// TestShutdownUsesSafeOrder troca componentes reais por funções que registram a
// sequência; assim testa a política sem abrir porta nem criar servidor.
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

// TestShutdownForcesWorkersWhenDeadlineEnds simula workers presos e prova que o
// cancelamento forçado ocorre quando o contexto já terminou.
func TestShutdownForcesWorkersWhenDeadlineEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var mutex sync.Mutex
	forced := false
	releaseWorkers := make(chan struct{})
	steps := Steps{
		MarkNotReady:   func() {},
		ShutdownHTTP:   func(context.Context) error { return context.Canceled },
		ForceCloseHTTP: func() error { return nil },
		CloseQueue:     func() {},
		WaitWorkers:    func() { <-releaseWorkers },
		ForceStopWorkers: func() {
			mutex.Lock()
			forced = true
			mutex.Unlock()
			close(releaseWorkers)
		},
	}
	if err := Shutdown(ctx, 0, steps); err == nil {
		t.Fatal("deadline encerrado deveria produzir erro")
	}
	mutex.Lock()
	defer mutex.Unlock()
	if !forced {
		t.Fatal("workers deveriam ser cancelados depois do prazo")
	}
}

// TestShutdownForcesHTTPWhenGracefulStopFails garante que conexões não fiquem
// abertas indefinidamente depois de Server.Shutdown devolver erro.
func TestShutdownForcesHTTPWhenGracefulStopFails(t *testing.T) {
	forcedHTTP := false
	steps := Steps{
		MarkNotReady:   func() {},
		ShutdownHTTP:   func(context.Context) error { return errors.New("HTTP preso") },
		ForceCloseHTTP: func() error { forcedHTTP = true; return nil },
		CloseQueue:     func() {},
		WaitWorkers:    func() {},
		ForceStopWorkers: func() {
			t.Fatal("workers não deveriam ser forçados sem deadline")
		},
	}
	if err := Shutdown(context.Background(), 0, steps); err == nil {
		t.Fatal("erro de ShutdownHTTP deveria ser preservado")
	}
	if !forcedHTTP {
		t.Fatal("ForceCloseHTTP deveria ser chamado")
	}
}

// TestShutdownRejectsMissingStep evita começar um encerramento impossível de
// completar quando o composition root esqueceu uma ação obrigatória.
func TestShutdownRejectsMissingStep(t *testing.T) {
	if err := Shutdown(context.Background(), 0, Steps{}); err == nil {
		t.Fatal("etapas ausentes deveriam ser rejeitadas")
	}
}
