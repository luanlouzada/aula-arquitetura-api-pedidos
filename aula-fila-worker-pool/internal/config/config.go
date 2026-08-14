// Package config concentra os limites usados pelo laboratório. Alterar um
// limite por variável de ambiente permite repetir a mesma carga com outra
// capacidade sem recompilar o programa. O restante da aplicação recebe uma
// Config pronta e não precisa consultar o ambiente diretamente.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config reúne as escolhas que determinam a capacidade do laboratório.
//
// Workers limita quantas goroutines processam lotes ao mesmo tempo.
// QueueCapacity limita quantos trabalhos podem aguardar no channel. BatchSize
// e BatchWait controlam quando um lote está pronto. FixedCost e PerJobCost são
// usados pelo processador simulado para tornar o efeito do lote mensurável.
// APIAddress define onde o servidor escuta e ShutdownTimeout limita quanto o
// encerramento pode esperar pela drenagem dos trabalhos aceitos.
type Config struct {
	APIAddress      string
	Workers         int
	QueueCapacity   int
	BatchSize       int
	BatchWait       time.Duration
	FixedCost       time.Duration
	PerJobCost      time.Duration
	ShutdownTimeout time.Duration
}

// Load lê as variáveis de ambiente, aplica os valores padrão e valida os
// limites antes que filas ou goroutines sejam criadas. Uma configuração
// inválida impede a inicialização; ela não é corrigida silenciosamente.
func Load() (Config, error) {
	workers, err := positiveInt("WORKERS", 4)
	if err != nil {
		return Config{}, err
	}
	queueCapacity, err := positiveInt("QUEUE_CAPACITY", 20)
	if err != nil {
		return Config{}, err
	}
	batchSize, err := positiveInt("BATCH_SIZE", 1)
	if err != nil {
		return Config{}, err
	}
	batchWait, err := duration("BATCH_WAIT", 50*time.Millisecond)
	if err != nil {
		return Config{}, err
	}
	fixedCost, err := duration("FIXED_COST", 80*time.Millisecond)
	if err != nil {
		return Config{}, err
	}
	perJobCost, err := duration("PER_JOB_COST", 20*time.Millisecond)
	if err != nil {
		return Config{}, err
	}
	shutdownTimeout, err := duration("SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}

	return Config{
		APIAddress:      valueOrDefault("API_ADDR", "127.0.0.1:8087"),
		Workers:         workers,
		QueueCapacity:   queueCapacity,
		BatchSize:       batchSize,
		BatchWait:       batchWait,
		FixedCost:       fixedCost,
		PerJobCost:      perJobCost,
		ShutdownTimeout: shutdownTimeout,
	}, nil
}

// positiveInt lê uma variável inteira que representa quantidade ou capacidade.
// fallback é usado somente quando a variável não foi definida. Zero, números
// negativos e textos que não sejam inteiros retornam erro.
func positiveInt(name string, fallback int) (int, error) {
	raw := valueOrDefault(name, strconv.Itoa(fallback))
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s deve ser um inteiro positivo: %q", name, raw)
	}
	return value, nil
}

// duration lê valores aceitos por time.ParseDuration, como "50ms" ou "2s".
// Duração zero é permitida; duração negativa ou texto inválido retorna erro.
func duration(name string, fallback time.Duration) (time.Duration, error) {
	raw := valueOrDefault(name, fallback.String())
	value, err := time.ParseDuration(raw)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("%s deve ser uma duração não negativa: %q", name, raw)
	}
	return value, nil
}

// valueOrDefault devolve o conteúdo da variável quando ela não está vazia.
// Caso contrário, devolve o valor padrão fornecido pelo chamador.
func valueOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
