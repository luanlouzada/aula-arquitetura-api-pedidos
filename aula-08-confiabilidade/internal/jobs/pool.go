package jobs

import (
	"context"
	"fmt"
	"sync"
	"time"

	"aula-08-confiabilidade/internal/telemetry"
)

// Processor representa o trabalho de negócio sem acoplar o Pool a uma
// implementação concreta. O contexto permite interrompê-lo após o prazo final.
type Processor func(context.Context, Job) error

// Pool cria exatamente workers goroutines de vida longa. Ele não cria uma nova
// goroutine por Job, portanto a concorrência do processamento possui teto.
type Pool struct {
	jobs      <-chan Job
	workers   int
	processor Processor
	metrics   *telemetry.Metrics
	wg        sync.WaitGroup
}

// NewPool valida e guarda as dependências, mas ainda não cria goroutines. Essa
// separação deixa o main controlar quando o processamento começa.
func NewPool(queue <-chan Job, workers int, processor Processor, metrics *telemetry.Metrics) (*Pool, error) {
	if queue == nil || processor == nil || metrics == nil {
		return nil, fmt.Errorf("queue, processor e metrics são obrigatórios")
	}
	if workers <= 0 {
		return nil, fmt.Errorf("workers deve ser positivo: %d", workers)
	}
	return &Pool{jobs: queue, workers: workers, processor: processor, metrics: metrics}, nil
}

// Start cria exatamente a quantidade configurada de workers e retorna. Cada
// goroutine processa um Job por vez; não nasce uma nova goroutine por requisição.
func (p *Pool) Start(ctx context.Context) {
	for range p.workers {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case job, open := <-p.jobs:
					if !open {
						return
					}
					p.metrics.RecordJobStarted(time.Since(job.EnqueuedAt))
					err := p.processor(ctx, job)
					p.metrics.RecordJobFinished(err)
				}
			}
		}()
	}
}

// Wait bloqueia até todas as goroutines do Pool saírem. No caminho gracioso,
// isso acontece depois que a fila fechada entrega todos os Jobs aceitos.
func (p *Pool) Wait() { p.wg.Wait() }

// NewSimulatedProcessor representa uma dependência lenta sem introduzir banco
// ou broker no laboratório. O timer respeita cancelamento no shutdown forçado.
func NewSimulatedProcessor(duration time.Duration) Processor {
	return func(ctx context.Context, _ Job) error {
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
