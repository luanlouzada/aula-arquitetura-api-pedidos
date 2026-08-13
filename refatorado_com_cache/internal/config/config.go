package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Este arquivo define quais configurações a API recebe do ambiente. Os valores
// padrão facilitam a execução local.

const (
	defaultDatabaseURL  = "postgres://postgres:postgres@localhost:5438/pedidos_cache?sslmode=disable"
	defaultRedisAddress = "127.0.0.1:6380"
	defaultAPIAddress   = "127.0.0.1:8084"
	defaultCacheTTL     = 30 * time.Second
	defaultCacheEnabled = true
)

// Config reúne os valores necessários para iniciar os mecanismos externos. Ela
// é consumida pelo composition root e não pelo domínio ou pelos casos de uso.
type Config struct {
	DatabaseURL  string
	RedisAddress string
	APIAddress   string
	CacheTTL     time.Duration
	CacheEnabled bool
}

// Load lê as variáveis de ambiente uma única vez durante a inicialização. Isso
// evita chamadas espalhadas a os.Getenv e deixa explícita a configuração da API.
func Load() (Config, error) {
	cacheTTL, err := requiredDuration("CACHE_TTL", defaultCacheTTL)
	if err != nil {
		return Config{}, err
	}
	cacheEnabled, err := optionalBool("CACHE_ENABLED", defaultCacheEnabled)
	if err != nil {
		return Config{}, err
	}
	settings := Config{
		DatabaseURL:  valueOrDefault("DATABASE_URL", defaultDatabaseURL),
		RedisAddress: valueOrDefault("REDIS_ADDR", defaultRedisAddress),
		APIAddress:   valueOrDefault("API_ADDR", defaultAPIAddress),
		CacheTTL:     cacheTTL,
		CacheEnabled: cacheEnabled,
	}
	if strings.TrimSpace(settings.DatabaseURL) == "" ||
		strings.TrimSpace(settings.RedisAddress) == "" ||
		strings.TrimSpace(settings.APIAddress) == "" {
		return Config{}, fmt.Errorf("DATABASE_URL, REDIS_ADDR e API_ADDR não podem ser vazios")
	}
	return settings, nil
}

// optionalBool aceita os valores compreendidos por strconv.ParseBool. A chave
// permite executar a mesma API sem o decorator de cache e obter uma linha de
// base para a comparação do laboratório.
func optionalBool(name string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	enabled, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s %q é inválido: use true ou false", name, value)
	}
	return enabled, nil
}

// valueOrDefault devolve o valor configurado ou o fallback de desenvolvimento
// quando a variável não existe ou foi definida como string vazia.
func valueOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

// requiredDuration aceita valores compreendidos por time.ParseDuration, como
// 500ms, 30s ou 5m. Valor inválido falha no startup para não executar a API com
// uma política de frescor diferente daquela pretendida pelo operador.
func requiredDuration(name string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%s %q é inválido: use uma duração positiva como 30s ou 5m", name, value)
	}
	return duration, nil
}
