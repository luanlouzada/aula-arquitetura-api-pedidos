package logging_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"aula-pedidos/refatorado_com_slog/internal/logging"
)

// TestLoggerWritesJSONWithCommonFields verifica o formato do logger: saída JSON
// e identificação do serviço e do ambiente em todos os registros.
func TestLoggerWritesJSONWithCommonFields(t *testing.T) {
	var output bytes.Buffer
	logger := logging.NewLogger(&output, "pedidos-test", "test", slog.LevelInfo)

	logger.Info("pedido criado", slog.Int64("order_id", 42))

	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("json.Unmarshal() error = %v; log = %s", err, output.String())
	}
	if record["service"] != "pedidos-test" {
		t.Fatalf("service = %v, want pedidos-test", record["service"])
	}
	if record["environment"] != "test" {
		t.Fatalf("environment = %v, want test", record["environment"])
	}
	if record["msg"] != "pedido criado" {
		t.Fatalf("msg = %v, want pedido criado", record["msg"])
	}
	if record["order_id"] != float64(42) {
		t.Fatalf("order_id = %v, want 42", record["order_id"])
	}
}

// TestLoggerFiltersLevels confirma que HandlerOptions controla o nível mínimo:
// com INFO, um registro DEBUG não deve ser escrito.
func TestLoggerFiltersLevels(t *testing.T) {
	var output bytes.Buffer
	logger := logging.NewLogger(&output, "pedidos-test", "test", slog.LevelInfo)

	logger.Debug("detalhe interno")

	if output.Len() != 0 {
		t.Fatalf("log DEBUG não deveria ser emitido: %s", output.String())
	}
}
