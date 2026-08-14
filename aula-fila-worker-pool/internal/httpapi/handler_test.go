package httpapi

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"aula-fila-worker-pool/internal/jobs"
	"aula-fila-worker-pool/internal/telemetry"
)

// TestEnqueueReturnsAccepted verifica o contrato de aceitação: com espaço na
// fila, POST /jobs responde 202. O teste não precisa iniciar workers porque a
// conclusão assíncrona não faz parte da resposta HTTP.
func TestEnqueueReturnsAccepted(t *testing.T) {
	handler, _ := newTestHandler(1)
	request := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewBufferString(`{"order_id":42}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d; esperado 202", response.Code)
	}
}

// TestEnqueueReturnsServiceUnavailableWhenQueueIsFull ocupa previamente o único
// espaço da fila. A próxima tentativa deve receber 503 e Retry-After, tornando a
// rejeição por capacidade um comportamento explícito da API.
func TestEnqueueReturnsServiceUnavailableWhenQueueIsFull(t *testing.T) {
	handler, queue := newTestHandler(1)
	_ = queue.TryEnqueue(jobs.Job{ID: "existing", OrderID: 1, EnqueuedAt: time.Now()})
	request := httptest.NewRequest(http.MethodPost, "/jobs", bytes.NewBufferString(`{"order_id":42}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; esperado 503", response.Code)
	}
	if got := response.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q; esperado 1", got)
	}
}

// newTestHandler monta somente a fronteira HTTP e suas dependências em memória.
// Nenhum worker é iniciado, permitindo que cada teste controle exatamente o
// conteúdo da fila.
func newTestHandler(capacity int) (http.Handler, *jobs.Queue) {
	queue := jobs.NewQueue(capacity)
	metrics := &telemetry.Metrics{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewHandler(queue, metrics, 1, logger).Routes(), queue
}
