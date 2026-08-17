package exports

import (
	"context"
	"fmt"
	"sync"
	"time"

	"exercicio-aula-08/internal/telemetry"
)

// Generator é o trabalho executado para uma Export. Usar uma função permite aos
// testes substituir a simulação por uma implementação rápida e observável.
type Generator func(context.Context, Export) error

// Pool mantém uma quantidade fixa de goroutines consumidoras. A quantidade de
// workers limita quantas exportações podem ser geradas simultaneamente.
type Pool struct {
	exports   <-chan Export
	workers   int
	generator Generator
	metrics   *telemetry.Metrics
	wg        sync.WaitGroup
}

// NewPool valida e guarda dependências; Start é quem efetivamente cria as
// goroutines, depois que o main terminou a montagem.
func NewPool(queue <-chan Export, workers int, generator Generator, metrics *telemetry.Metrics) (*Pool, error) {
	if queue == nil || generator == nil || metrics == nil {
		return nil, fmt.Errorf("queue, generator e metrics são obrigatórios")
	}
	if workers <= 0 {
		return nil, fmt.Errorf("workers deve ser positivo: %d", workers)
	}
	return &Pool{exports: queue, workers: workers, generator: generator, metrics: metrics}, nil
}

// Start cria exatamente workers goroutines de vida longa. Cada uma processa um
// item por vez e volta a esperar; nenhuma goroutine extra nasce por Export.
func (p *Pool) Start(ctx context.Context) {
	for range p.workers {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case item, open := <-p.exports:
					if !open {
						return
					}
					p.metrics.RecordExportStarted(time.Since(item.EnqueuedAt))
					err := p.generator(ctx, item)
					p.metrics.RecordExportFinished(err)
				}
			}
		}()
	}
}

// Wait retorna somente depois que todos os workers saíram por fila drenada ou
// cancelamento forçado do contexto.
func (p *Pool) Wait() { p.wg.Wait() }

// NewSimulatedGenerator representa uma geração lenta com um timer cancelável.
// Não cria arquivo: o atraso basta para tornar capacidade e drenagem visíveis.
func NewSimulatedGenerator(duration time.Duration) Generator {
	return func(ctx context.Context, _ Export) error {
		timer := time.NewTimer(duration)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		}
	}
}
