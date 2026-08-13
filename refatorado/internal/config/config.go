package config

import "os"

// Este arquivo define quais configurações a API recebe do ambiente. Os valores
// padrão facilitam a execução local.

const (
	defaultDatabaseURL = "postgres://postgres:postgres@localhost:5435/pedidos?sslmode=disable"
	defaultAPIAddress  = "127.0.0.1:8081"
)

// Config reúne os valores necessários para iniciar os mecanismos externos. Ela
// é consumida pelo composition root e não pelo domínio ou pelos casos de uso.
type Config struct {
	DatabaseURL string
	APIAddress  string
}

// Load lê as variáveis de ambiente uma única vez durante a inicialização. Isso
// evita chamadas espalhadas a os.Getenv e deixa explícita a configuração da API.
func Load() Config {
	return Config{
		DatabaseURL: valueOrDefault("DATABASE_URL", defaultDatabaseURL),
		APIAddress:  valueOrDefault("API_ADDR", defaultAPIAddress),
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
