// Package logging concentra a política de logs estruturados da API. Ele monta
// o logger e acrescenta registros ao redor dos casos de uso sem decidir regras
// de Order, códigos HTTP ou comandos SQL.
package logging

import (
	"io"
	"log/slog"
)

// NewLogger cria o logger compartilhado pela API.
//
// JSONHandler transforma cada registro em um objeto JSON, HandlerOptions
// aplica o nível mínimo e With adiciona service e environment a todas as
// linhas. output é recebido como interface para permitir stdout em execução e
// bytes.Buffer nos testes.
func NewLogger(output io.Writer, serviceName, environment string, level slog.Level) *slog.Logger {
	handler := slog.NewJSONHandler(output, &slog.HandlerOptions{Level: level})
	return slog.New(handler).With(
		slog.String("service", serviceName),
		slog.String("environment", environment),
	)
}
