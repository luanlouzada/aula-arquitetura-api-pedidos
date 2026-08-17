// Package config lê e valida os limites usados pela central de exportações.
// Nenhum handler consulta os.Getenv: a configuração é montada uma vez no início.
package config

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"time"
)

// Config reúne endereço, identidade e limites de capacidade. Cada instância
// recebe sua própria cópia destes valores e, portanto, seu próprio orçamento.
type Config struct {
	APIAddress      string
	InstanceID      string
	Workers         int
	QueueCapacity   int
	GenerationTime  time.Duration
	RateLimitRPS    float64
	RateLimitBurst  int
	DrainDelay      time.Duration
	ShutdownTimeout time.Duration
}

// Load aplica padrões adequados ao laboratório e falha cedo se uma variável não
// puder representar o tipo ou o intervalo esperado.
func Load() (Config, error) {
	workers, err := positiveInt("WORKERS", 2)
	if err != nil {
		return Config{}, err
	}
	queueCapacity, err := positiveInt("QUEUE_CAPACITY", 10)
	if err != nil {
		return Config{}, err
	}
	generationTime, err := nonNegativeDuration("GENERATION_TIME", 250*time.Millisecond)
	if err != nil {
		return Config{}, err
	}
	rateLimitRPS, err := positiveFloat("RATE_LIMIT_RPS", 8)
	if err != nil {
		return Config{}, err
	}
	rateLimitBurst, err := positiveInt("RATE_LIMIT_BURST", 4)
	if err != nil {
		return Config{}, err
	}
	drainDelay, err := nonNegativeDuration("DRAIN_DELAY", time.Second)
	if err != nil {
		return Config{}, err
	}
	shutdownTimeout, err := positiveDuration("SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	return Config{
		APIAddress:      valueOrDefault("API_ADDR", "127.0.0.1:8090"),
		InstanceID:      valueOrDefault("INSTANCE_ID", hostname()),
		Workers:         workers,
		QueueCapacity:   queueCapacity,
		GenerationTime:  generationTime,
		RateLimitRPS:    rateLimitRPS,
		RateLimitBurst:  rateLimitBurst,
		DrainDelay:      drainDelay,
		ShutdownTimeout: shutdownTimeout,
	}, nil
}

// positiveInt lê quantidades que precisam ser maiores que zero, como workers.
func positiveInt(name string, fallback int) (int, error) {
	raw := valueOrDefault(name, strconv.Itoa(fallback))
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s deve ser um inteiro positivo: %q", name, raw)
	}
	return value, nil
}

// positiveFloat aceita taxas fracionárias; por exemplo, 0.5 permissão por
// segundo equivale a uma permissão a cada dois segundos. Valores especiais não
// finitos também são recusados antes de chegar ao TODO do token bucket.
func positiveFloat(name string, fallback float64) (float64, error) {
	raw := valueOrDefault(name, strconv.FormatFloat(fallback, 'f', -1, 64))
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("%s deve ser um número positivo e finito: %q", name, raw)
	}
	return value, nil
}

// nonNegativeDuration permite zero quando não esperar é uma configuração válida.
func nonNegativeDuration(name string, fallback time.Duration) (time.Duration, error) {
	raw := valueOrDefault(name, fallback.String())
	value, err := time.ParseDuration(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s deve ser uma duração não negativa: %q", name, raw)
	}
	return value, nil
}

// positiveDuration é usada quando zero eliminaria o prazo operacional necessário.
func positiveDuration(name string, fallback time.Duration) (time.Duration, error) {
	value, err := nonNegativeDuration(name, fallback)
	if err != nil || value == 0 {
		if err != nil {
			return 0, err
		}
		return 0, fmt.Errorf("%s deve ser maior que zero", name)
	}
	return value, nil
}

// valueOrDefault devolve fallback somente quando a variável está vazia.
func valueOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

// hostname fornece uma identidade local quando INSTANCE_ID não foi configurado.
// O Compose usa nomes explícitos para distinguir cada exportador.
func hostname() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "local"
	}
	return name
}
