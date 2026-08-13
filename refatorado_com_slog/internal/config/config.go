package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

const (
	defaultDatabaseURL = "postgres://postgres:postgres@localhost:5437/pedidos_slog?sslmode=disable"
	defaultAPIAddress  = "127.0.0.1:8083"
	defaultServiceName = "pedidos-api"
	defaultEnvironment = "development"
	defaultLogLevel    = "info"
)

// Config reúne as decisões externas usadas para montar o processo. Domínio e
// aplicação não recebem Config e não conhecem variáveis de ambiente.
type Config struct {
	DatabaseURL string
	APIAddress  string
	ServiceName string
	Environment string
	LogLevel    slog.Level
}

// Load lê e valida o ambiente antes de a API começar a atender requisições.
// Assim um LOG_LEVEL incorreto, por exemplo, falha durante o startup em vez de
// produzir silenciosamente uma política de logs diferente da esperada.
func Load() (Config, error) {
	levelText := valueOrDefault("LOG_LEVEL", defaultLogLevel)
	var level slog.Level
	if err := level.UnmarshalText([]byte(levelText)); err != nil {
		return Config{}, fmt.Errorf("LOG_LEVEL %q é inválido: use debug, info, warn ou error", levelText)
	}

	settings := Config{
		DatabaseURL: valueOrDefault("DATABASE_URL", defaultDatabaseURL),
		APIAddress:  valueOrDefault("API_ADDR", defaultAPIAddress),
		ServiceName: valueOrDefault("SERVICE_NAME", defaultServiceName),
		Environment: valueOrDefault("APP_ENV", defaultEnvironment),
		LogLevel:    level,
	}

	// Campos essenciais vazios normalmente representam uma implantação mal
	// configurada. Interromper aqui produz um erro mais fácil de diagnosticar.
	if strings.TrimSpace(settings.DatabaseURL) == "" ||
		strings.TrimSpace(settings.APIAddress) == "" ||
		strings.TrimSpace(settings.ServiceName) == "" ||
		strings.TrimSpace(settings.Environment) == "" {
		return Config{}, fmt.Errorf("DATABASE_URL, API_ADDR, SERVICE_NAME e APP_ENV não podem ser vazios")
	}
	return settings, nil
}

// valueOrDefault devolve o valor configurado ou o padrão local quando a
// variável não existe, está vazia ou contém somente espaços.
func valueOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
