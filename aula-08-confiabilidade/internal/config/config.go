// Package config traduz variáveis de ambiente para escolhas explícitas de
// capacidade. O restante do programa recebe uma Config validada e não consulta
// o ambiente diretamente.
package config

import (
	"fmt"
	"math"
	"os"
	"strconv"
	"time"
)

// Config é a fotografia imutável das escolhas feitas na inicialização. Manter
// esses valores juntos evita que packages internos leiam variáveis de ambiente
// durante uma requisição e torna explícito quais números limitam o serviço.
type Config struct {
	APIAddress      string
	InstanceID      string
	Workers         int
	QueueCapacity   int
	ProcessingTime  time.Duration
	RateLimitRPS    float64
	RateLimitBurst  int
	DrainDelay      time.Duration
	ShutdownTimeout time.Duration
}

// Load lê todas as variáveis uma única vez, aplica padrões para o laboratório e
// falha cedo quando um valor é inválido. É melhor impedir a inicialização do que
// executar com uma capacidade diferente da imaginada pelo operador.
func Load() (Config, error) {
	workers, err := positiveInt("WORKERS", 2)
	if err != nil {
		return Config{}, err
	}
	queueCapacity, err := positiveInt("QUEUE_CAPACITY", 10)
	if err != nil {
		return Config{}, err
	}
	processingTime, err := nonNegativeDuration("PROCESSING_TIME", 250*time.Millisecond)
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
		APIAddress:      valueOrDefault("API_ADDR", ":8080"),
		InstanceID:      valueOrDefault("INSTANCE_ID", hostname()),
		Workers:         workers,
		QueueCapacity:   queueCapacity,
		ProcessingTime:  processingTime,
		RateLimitRPS:    rateLimitRPS,
		RateLimitBurst:  rateLimitBurst,
		DrainDelay:      drainDelay,
		ShutdownTimeout: shutdownTimeout,
	}, nil
}

// positiveInt lê quantidades que nunca podem ser zero, como workers e tamanho
// da fila. fallback só é usado quando a variável não foi definida.
func positiveInt(name string, fallback int) (int, error) {
	raw := valueOrDefault(name, strconv.Itoa(fallback))
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s deve ser um inteiro positivo: %q", name, raw)
	}
	return value, nil
}

// positiveFloat existe porque a taxa pode ter parte decimal: 0.5 significa uma
// permissão a cada dois segundos. Além de positiva, ela precisa ser finita;
// ParseFloat aceita representações especiais como NaN e +Inf.
func positiveFloat(name string, fallback float64) (float64, error) {
	raw := valueOrDefault(name, strconv.FormatFloat(fallback, 'f', -1, 64))
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("%s deve ser um número positivo e finito: %q", name, raw)
	}
	return value, nil
}

// nonNegativeDuration aceita zero nos tempos em que "não esperar" é uma escolha
// válida, como DRAIN_DELAY=0 em um teste local.
func nonNegativeDuration(name string, fallback time.Duration) (time.Duration, error) {
	raw := valueOrDefault(name, fallback.String())
	value, err := time.ParseDuration(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s deve ser uma duração não negativa: %q", name, raw)
	}
	return value, nil
}

// positiveDuration rejeita zero quando a ausência de prazo poderia deixar o
// encerramento sem uma janela útil para terminar.
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

// valueOrDefault diferencia variável ausente de variável preenchida. Uma string
// vazia usa o padrão, mantendo a regra de fallback centralizada.
func valueOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

// hostname fornece uma identidade observável quando INSTANCE_ID não foi
// definido. Em Compose, nomes explícitos como api-1 distinguem as réplicas.
func hostname() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "local"
	}
	return name
}
