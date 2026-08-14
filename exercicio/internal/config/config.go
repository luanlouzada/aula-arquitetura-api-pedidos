// Package config concentra os limites da cozinha. Alterar um valor por
// variável de ambiente permite repetir a mesma carga com outra capacidade
// sem recompilar o programa.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config reúne as escolhas que determinam a capacidade da cozinha.
//
// Cooks limita quantas goroutines preparam pratos ao mesmo tempo.
// QueueCapacity limita quantas comandas podem esperar na fila.
// FixedCost e PerTicketCost definem quanto tempo o preparo simulado leva.
type Config struct {
	APIAddress      string
	Cooks           int
	QueueCapacity   int
	FixedCost       time.Duration
	PerTicketCost   time.Duration
	ShutdownTimeout time.Duration
}

// Load lê as variáveis de ambiente, aplica os valores padrão e valida os
// limites antes que a fila ou os cozinheiros sejam criados.
func Load() (Config, error) {
	cooks, err := positiveInt("COOKS", 4)
	if err != nil {
		return Config{}, err
	}
	queueCapacity, err := positiveInt("QUEUE_CAPACITY", 20)
	if err != nil {
		return Config{}, err
	}
	fixedCost, err := duration("FIXED_COST", 80*time.Millisecond)
	if err != nil {
		return Config{}, err
	}
	perTicketCost, err := duration("PER_TICKET_COST", 20*time.Millisecond)
	if err != nil {
		return Config{}, err
	}
	shutdownTimeout, err := duration("SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}

	return Config{
		APIAddress:      valueOrDefault("API_ADDR", "127.0.0.1:8088"),
		Cooks:           cooks,
		QueueCapacity:   queueCapacity,
		FixedCost:       fixedCost,
		PerTicketCost:   perTicketCost,
		ShutdownTimeout: shutdownTimeout,
	}, nil
}

func positiveInt(name string, fallback int) (int, error) {
	raw := valueOrDefault(name, strconv.Itoa(fallback))
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s deve ser um inteiro positivo: %q", name, raw)
	}
	return value, nil
}

func duration(name string, fallback time.Duration) (time.Duration, error) {
	raw := valueOrDefault(name, fallback.String())
	value, err := time.ParseDuration(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s deve ser uma duração não negativa: %q", name, raw)
	}
	return value, nil
}

func valueOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
