package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aula-08-confiabilidade/internal/health"
	"aula-08-confiabilidade/internal/jobs"
	"aula-08-confiabilidade/internal/telemetry"
)

// fakeGate permite escolher admissão ou rejeição sem esperar o relógio real.
type fakeGate struct {
	allowed bool
}

// Allow devolve a decisão preparada pelo teste.
func (g fakeGate) Allow() bool { return g.allowed }

// Tokens satisfaz o contrato usado por /stats; saldo não importa nestes testes.
func (g fakeGate) Tokens() float64 { return 0 }

// TestEnqueueReturnsAcceptedWhenReadyAndAdmitted cobre o caminho feliz até 202.
func TestEnqueueReturnsAcceptedWhenReadyAndAdmitted(t *testing.T) {
	handler, _, state := newTestHandler(t, true, 1)
	state.MarkReady()
	response := postJob(handler)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d; esperado 202", response.Code)
	}
	if got := response.Header().Get("X-Instance-ID"); got != "api-test" {
		t.Fatalf("X-Instance-ID = %q; esperado api-test", got)
	}
}

// TestEnqueueReturns503WhenInstanceIsNotReady confirma que drain barra a
// operação antes de consumir token ou tentar a fila.
func TestEnqueueReturns503WhenInstanceIsNotReady(t *testing.T) {
	handler, _, _ := newTestHandler(t, true, 1)
	response := postJob(handler)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; esperado 503", response.Code)
	}
	if got := response.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q; esperado 1", got)
	}
	if !strings.Contains(response.Body.String(), "not_ready") {
		t.Fatalf("corpo deveria explicar reason=not_ready: %s", response.Body.String())
	}
}

// TestEnqueueReturns400ForInvalidOrder prova que validação acontece antes do
// token bucket e não transforma erro do cliente em sobrecarga.
func TestEnqueueReturns400ForInvalidOrder(t *testing.T) {
	handler, _, state := newTestHandler(t, false, 1)
	state.MarkReady()
	request := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewBufferString(`{"order_id":0}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; esperado 400", response.Code)
	}
}

// TestEnqueueReturns429WhenTokenBucketRejects fixa a tradução de admissão para HTTP.
func TestEnqueueReturns429WhenTokenBucketRejects(t *testing.T) {
	handler, _, state := newTestHandler(t, false, 1)
	state.MarkReady()
	response := postJob(handler)
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d; esperado 429", response.Code)
	}
	if got := response.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q; esperado 1", got)
	}
}

// TestEnqueueReturns503WhenQueueIsFull distingue capacidade de espera de rate limit.
func TestEnqueueReturns503WhenQueueIsFull(t *testing.T) {
	handler, queue, state := newTestHandler(t, true, 1)
	state.MarkReady()
	_ = queue.TryEnqueue(jobs.Job{ID: "existing"})
	response := postJob(handler)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; esperado 503", response.Code)
	}
}

// TestEnqueueReturns503WhenQueueClosedDuringShutdown garante que um handler
// atrasado recebe uma resposta operacional em vez de causar panic.
func TestEnqueueReturns503WhenQueueClosedDuringShutdown(t *testing.T) {
	handler, queue, state := newTestHandler(t, true, 1)
	state.MarkReady()
	queue.Close()

	response := postJob(handler)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; esperado 503", response.Code)
	}
	if !strings.Contains(response.Body.String(), "not_ready") {
		t.Fatalf("corpo deveria explicar reason=not_ready: %s", response.Body.String())
	}
}

// TestReadinessChangesButLivenessStaysHealthy prova que drain não significa morte.
func TestReadinessChangesButLivenessStaysHealthy(t *testing.T) {
	handler, _, state := newTestHandler(t, true, 1)
	if got := get(handler, "/readyz").Code; got != http.StatusServiceUnavailable {
		t.Fatalf("readiness inicial = %d; esperado 503", got)
	}
	if got := get(handler, "/livez").Code; got != http.StatusOK {
		t.Fatalf("liveness inicial = %d; esperado 200", got)
	}
	state.MarkReady()
	if got := get(handler, "/readyz").Code; got != http.StatusOK {
		t.Fatalf("readiness pronta = %d; esperado 200", got)
	}
	state.MarkNotReady()
	if got := get(handler, "/livez").Code; got != http.StatusOK {
		t.Fatalf("liveness durante drain = %d; esperado 200", got)
	}
}

// newTestHandler monta somente componentes em memória e devolve Queue e State
// para que cada teste controle as pré-condições.
func newTestHandler(t *testing.T, allowed bool, capacity int) (http.Handler, *jobs.Queue, *health.State) {
	t.Helper()
	queue := jobs.NewQueue(capacity)
	state := health.NewState()
	metrics := telemetry.New("api-test")
	handler := NewHandler("api-test", state, fakeGate{allowed: allowed}, queue, metrics, 2, 8, 4)
	return handler.Routes(), queue, state
}

// postJob envia o mesmo payload válido e devolve a resposta capturada em memória.
func postJob(handler http.Handler) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewBufferString(`{"order_id":42}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

// get exercita uma rota GET sem abrir uma porta TCP real.
func get(handler http.Handler, path string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
