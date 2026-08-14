package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"exercicio/internal/kitchen"
)

func TestEnqueueReturnsAccepted(t *testing.T) {
	handler, _ := newTestHandler(1)
	request := httptest.NewRequest(http.MethodPost, "/tickets", bytes.NewBufferString(`{"prato":"lasanha"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("status = %d; esperado 202", response.Code)
	}
}

func TestEnqueueReturnsServiceUnavailableWhenQueueIsFull(t *testing.T) {
	handler, queue := newTestHandler(1)
	if err := queue.TryEnqueue(kitchen.Ticket{ID: "existing", Dish: "pizza", EnqueuedAt: time.Now()}); err != nil {
		t.Fatalf("preparar fila cheia: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/tickets", bytes.NewBufferString(`{"prato":"lasanha"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d; esperado 503", response.Code)
	}
	if got := response.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q; esperado 1", got)
	}
}

func TestEnqueueRejectsEmptyDish(t *testing.T) {
	handler, _ := newTestHandler(1)
	request := httptest.NewRequest(http.MethodPost, "/tickets", bytes.NewBufferString(`{"prato":""}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; esperado 400", response.Code)
	}
}

func newTestHandler(capacity int) (http.Handler, *kitchen.Queue) {
	queue := kitchen.NewQueue(capacity)
	return NewHandler(queue).Routes(), queue
}
