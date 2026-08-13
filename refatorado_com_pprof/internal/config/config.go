package config

import "os"

// Este arquivo define quais configurações a API e o servidor administrativo
// recebem do ambiente. Os valores padrão facilitam a execução local.

const (
	defaultDatabaseURL  = "postgres://postgres:postgres@localhost:5437/pedidos_pprof?sslmode=disable"
	defaultAPIAddress   = "127.0.0.1:8083"
	defaultPprofAddress = "127.0.0.1:6060"
)

// Config reúne os valores necessários para iniciar a API, o PostgreSQL e a
// interface administrativa de profiling. Domínio e aplicação não conhecem
// variáveis de ambiente nem os endpoints do pprof.
type Config struct {
	DatabaseURL  string
	APIAddress   string
	PprofAddress string
}

// Load lê o ambiente uma única vez no composition root. PPROF_ADDR permanece
// separado de API_ADDR para que os endpoints de diagnóstico não sejam
// publicados junto das rotas de negócio.
func Load() Config {
	return Config{
		DatabaseURL:  valueOrDefault("DATABASE_URL", defaultDatabaseURL),
		APIAddress:   valueOrDefault("API_ADDR", defaultAPIAddress),
		PprofAddress: valueOrDefault("PPROF_ADDR", defaultPprofAddress),
	}
}

// valueOrDefault devolve o valor configurado ou o fallback de desenvolvimento
// quando a variável não existe ou foi definida como string vazia.
func valueOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
