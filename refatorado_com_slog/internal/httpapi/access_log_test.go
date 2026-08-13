package httpapi_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"aula-pedidos/refatorado_com_slog/internal/httpapi"
)

// TestAccessLogSeparatesMethodAndRoute garante campos estáveis: o método fica
// em http.method e o template do caminho fica em http.route, sem IDs concretos.
func TestAccessLogSeparatesMethodAndRoute(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	router := http.NewServeMux()
	router.HandleFunc("GET /orders/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	request := httptest.NewRequest(http.MethodGet, "/orders/42", nil)
	response := httptest.NewRecorder()
	httpapi.AccessLog(logger, router).ServeHTTP(response, request)

	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; log = %s", err, output.String())
	}
	if got, want := record["http.method"], http.MethodGet; got != want {
		t.Fatalf("http.method = %v, want %v", got, want)
	}
	if got, want := record["http.route"], "/orders/{id}"; got != want {
		t.Fatalf("http.route = %v, want %v", got, want)
	}
	if got, want := record["http.status_code"], float64(http.StatusNoContent); got != want {
		t.Fatalf("http.status_code = %v, want %v", got, want)
	}
}
