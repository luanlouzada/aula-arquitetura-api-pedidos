package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"exercicio-aula-08/internal/exports"
	"exercicio-aula-08/internal/health"
	"exercicio-aula-08/internal/telemetry"
)

// fakeGate deixa o teste escolher a resposta de admissão sem usar tempo real.
type fakeGate struct{ allowed bool }

// Allow devolve a decisão preparada pelo cenário.
func (g fakeGate) Allow() bool { return g.allowed }

// Tokens existe para satisfazer Gate; /stats não é o foco destes testes.
func (g fakeGate) Tokens() float64 { return 0 }

// TestEnqueueReturns503WhenInstanceIsNotReady fixa a primeira barreira do fluxo.
func TestEnqueueReturns503WhenInstanceIsNotReady(t *testing.T) {
	handler, _, _ := newTestHandler(false, 1)
	response := postExport(handler)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; esperado 503", response.Code)
	}
	if !strings.Contains(response.Body.String(), "not_ready") {
		t.Fatalf("resposta deveria explicar reason=not_ready: %s", response.Body.String())
	}
	if got := response.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q; esperado 1", got)
	}
}

// TestEnqueueReturns429WhenAdmissionRejects verifica status e orientação de retry.
func TestEnqueueReturns429WhenAdmissionRejects(t *testing.T) {
	handler, _, state := newTestHandler(false, 1)
	state.MarkReady()
	response := postExport(handler)
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d; esperado 429", response.Code)
	}
	if got := response.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q; esperado 1", got)
	}
}

// TestEnqueueReturns202WhenReadyAndAdmitted cobre o caminho de aceitação.
func TestEnqueueReturns202WhenReadyAndAdmitted(t *testing.T) {
	handler, _, state := newTestHandler(true, 1)
	state.MarkReady()
	response := postExport(handler)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d; esperado 202", response.Code)
	}
}

// TestEnqueueReturns400ForEmptyReport garante que entrada inválida não consome
// a decisão preparada de rate limit.
func TestEnqueueReturns400ForEmptyReport(t *testing.T) {
	handler, _, state := newTestHandler(false, 1)
	state.MarkReady()
	request := httptest.NewRequest(http.MethodPost, "/exports", bytes.NewBufferString(`{"report":""}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; esperado 400", response.Code)
	}
}

// TestEnqueueReturns503WhenQueueIsFull separa fila cheia de token indisponível.
func TestEnqueueReturns503WhenQueueIsFull(t *testing.T) {
	handler, queue, state := newTestHandler(true, 1)
	state.MarkReady()
	_ = queue.TryEnqueue(exports.Export{ID: "existing"})
	response := postExport(handler)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; esperado 503", response.Code)
	}
	if !strings.Contains(response.Body.String(), "queue_full") {
		t.Fatalf("resposta deveria explicar reason=queue_full: %s", response.Body.String())
	}
}

// TestProvidedQueueCloseBecomes503 garante que a proteção de infraestrutura
// fornecida ao exercício não transforma shutdown concorrente em panic ou 500.
func TestProvidedQueueCloseBecomes503(t *testing.T) {
	handler, queue, state := newTestHandler(true, 1)
	state.MarkReady()
	queue.Close()

	response := postExport(handler)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; esperado 503", response.Code)
	}
	if !strings.Contains(response.Body.String(), "not_ready") {
		t.Fatalf("resposta deveria explicar reason=not_ready: %s", response.Body.String())
	}
}

// TestLivenessStays200WhileReadinessIs503 prova que não pronto não significa morto.
func TestLivenessStays200WhileReadinessIs503(t *testing.T) {
	handler, _, _ := newTestHandler(true, 1)
	if got := get(handler, "/livez").Code; got != http.StatusOK {
		t.Fatalf("liveness = %d; esperado 200", got)
	}
	if got := get(handler, "/readyz").Code; got != http.StatusServiceUnavailable {
		t.Fatalf("readiness = %d; esperado 503", got)
	}
}

// newTestHandler monta dependências em memória e as devolve para o teste preparar
// prontidão e ocupação da fila.
func newTestHandler(allowed bool, capacity int) (http.Handler, *exports.Queue, *health.State) {
	queue := exports.NewQueue(capacity)
	state := health.NewState()
	metrics := telemetry.New("exporter-test")
	handler := NewHandler("exporter-test", state, fakeGate{allowed: allowed}, queue, metrics, 2, 8, 4)
	return handler.Routes(), queue, state
}

// postExport envia um corpo válido e captura a resposta sem rede real.
func postExport(handler http.Handler) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/exports", bytes.NewBufferString(`{"report":"sales-2026"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

// get exercita uma probe usando httptest.ResponseRecorder.
func get(handler http.Handler, path string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
