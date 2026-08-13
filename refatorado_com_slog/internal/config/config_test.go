package config_test

import (
	"log/slog"
	"strings"
	"testing"

	"aula-pedidos/refatorado_com_slog/internal/config"
)

// TestLoadConvertsLogLevel confirma que a camada de configuração entrega um
// slog.Level pronto; os outros componentes não precisam interpretar strings.
func TestLoadConvertsLogLevel(t *testing.T) {
	t.Setenv("LOG_LEVEL", "warn")

	settings, err := config.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if settings.LogLevel != slog.LevelWarn {
		t.Fatalf("LogLevel = %v, want %v", settings.LogLevel, slog.LevelWarn)
	}
}

// TestLoadRejectsInvalidLogLevel prova o fail fast: um valor desconhecido não
// deixa a API começar com um nível diferente do pretendido.
func TestLoadRejectsInvalidLogLevel(t *testing.T) {
	t.Setenv("LOG_LEVEL", "verbose")

	_, err := config.Load()

	if err == nil || !strings.Contains(err.Error(), "LOG_LEVEL") {
		t.Fatalf("Load() error = %v, want LOG_LEVEL error", err)
	}
}
