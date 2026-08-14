package jobs

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"aula-fila-worker-pool/internal/telemetry"
)

// Processor representa o trabalho executado fora do mecanismo da fila. Ele
// recebe o contexto de cancelamento e um lote que contém pelo menos um Job.
// Uma implementação real poderia executar um INSERT em lote, enviar uma
// requisição bulk, isto é, vários itens em uma chamada, ou acessar outro
// mecanismo externo.
//
// Usar um tipo de função permite trocar o processamento simulado por outro
// comportamento sem alterar Pool. Os testes também podem fornecer uma função
// pequena que apenas registra os lotes recebidos.
type Processor func(ctx context.Context, batch []Job) error

// Pool mantém uma quantidade fixa de workers consumindo o mesmo channel.
//
// jobs é somente de leitura: Pool consome, mas não controla o fechamento da
// fila. O WaitGroup permite aguardar todas as goroutines durante o shutdown.
// Os demais campos definem como cada worker forma e processa seus lotes.
type Pool struct {
	jobs      <-chan Job
	workers   int
	batchSize int
	batchWait time.Duration
	processor Processor
	metrics   *telemetry.Metrics
	logger    *slog.Logger
	wg        sync.WaitGroup
}

// NewPool valida e reúne as dependências do worker pool. Ele ainda não inicia
// goroutines; Start realiza essa etapa depois que toda a aplicação foi montada.
func NewPool(
	queue <-chan Job,
	workers int,
	batchSize int,
	batchWait time.Duration,
	processor Processor,
	metrics *telemetry.Metrics,
	logger *slog.Logger,
) (*Pool, error) {
	if queue == nil || processor == nil || metrics == nil {
		return nil, errors.New("queue, processor e metrics são obrigatórios")
	}
	if workers <= 0 || batchSize <= 0 || batchWait < 0 {
		return nil, errors.New("workers e batchSize devem ser positivos; batchWait não pode ser negativo")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Pool{
		jobs:      queue,
		workers:   workers,
		batchSize: batchSize,
		batchWait: batchWait,
		processor: processor,
		metrics:   metrics,
		logger:    logger,
	}, nil
}

// Start cria uma quantidade fixa de goroutines consumidoras. Todas disputam a
// mesma fila e o runtime entrega cada Job para apenas uma delas. Limitar a
// quantidade de workers limita também quantos lotes podem chamar Processor ao
// mesmo tempo.
func (p *Pool) Start(ctx context.Context) {
	for workerID := 1; workerID <= p.workers; workerID++ {
		// Add acontece antes de iniciar a goroutine para que Wait nunca observe
		// um contador incompleto.
		p.wg.Add(1)
		go func(id int) {
			defer p.wg.Done()
			p.runWorker(ctx, id)
		}(workerID)
	}
}

// Wait bloqueia até que todos os workers tenham saído. Isso ocorre quando a
// fila é fechada e drenada ou quando o contexto do pool é cancelado.
func (p *Pool) Wait() {
	p.wg.Wait()
}

// runWorker executa o ciclo de vida de um consumidor. Primeiro espera pelo item
// que inicia o próximo lote; depois coleta itens adicionais, processa o lote e
// volta a esperar. Receber ok=false significa que o channel foi fechado.
func (p *Pool) runWorker(ctx context.Context, workerID int) {
	for {
		var first Job
		var ok bool
		select {
		case <-ctx.Done():
			return
		case first, ok = <-p.jobs:
			if !ok {
				return
			}
		}

		// O primeiro item já saiu da fila. collectBatch tenta completar o lote
		// sem obrigar tráfego pequeno a esperar indefinidamente.
		batch, queueClosed := p.collectBatch(ctx, first)
		p.processBatch(ctx, workerID, batch)
		// Se o channel fechou durante a coleta, o lote parcial acima ainda é
		// processado antes de o worker encerrar.
		if queueClosed {
			return
		}
	}
}

// collectBatch forma um lote a partir de first e o libera quando atinge
// batchSize ou quando batchWait termina. Assim, uma rajada aproveita lotes
// cheios, enquanto pouco tráfego não espera indefinidamente por novos itens.
//
// O bool retornado informa se a fila ou o contexto encerrou durante a coleta.
// Nesse caso, runWorker processa o lote parcial e depois termina.
func (p *Pool) collectBatch(ctx context.Context, first Job) ([]Job, bool) {
	batch := make([]Job, 0, p.batchSize)
	batch = append(batch, first)
	if p.batchSize == 1 {
		return batch, false
	}

	// O timer começa quando o primeiro item do lote já foi obtido. Portanto,
	// BatchWait limita a espera adicional para completar esse lote.
	timer := time.NewTimer(p.batchWait)
	defer stopTimer(timer)
	for len(batch) < p.batchSize {
		select {
		case <-ctx.Done():
			return batch, true
		case job, ok := <-p.jobs:
			if !ok {
				return batch, true
			}
			batch = append(batch, job)
		case <-timer.C:
			return batch, false
		}
	}
	return batch, false
}

// processBatch mede a espera acumulada dos itens, executa Processor uma única
// vez e registra o resultado do lote. Um erro marca todos os itens daquele lote
// como falhos; retry e DLQ não fazem parte deste laboratório em memória.
func (p *Pool) processBatch(ctx context.Context, workerID int, batch []Job) {
	startedAt := time.Now()
	var totalQueueWait time.Duration
	for _, job := range batch {
		totalQueueWait += startedAt.Sub(job.EnqueuedAt)
	}
	p.metrics.RecordBatchStart(len(batch), totalQueueWait)

	err := p.processor(ctx, batch)
	elapsed := time.Since(startedAt)
	p.metrics.RecordBatchEnd(len(batch), elapsed, err)
	if err != nil {
		p.logger.Error("lote falhou",
			slog.Int("worker_id", workerID),
			slog.Int("batch_size", len(batch)),
			slog.Duration("duration", elapsed),
			slog.Any("error", err),
		)
		return
	}
	// O contador mede todos os lotes; o log por lote fica em DEBUG para evitar
	// volume proporcional ao tráfego durante uma rajada.
	p.logger.Debug("lote processado",
		slog.Int("worker_id", workerID),
		slog.Int("batch_size", len(batch)),
		slog.Duration("duration", elapsed),
	)
}

// stopTimer interrompe um timer que ainda não disparou. Se o disparo já estiver
// disponível, a leitura não bloqueante esvazia o channel do timer. Esse padrão
// evita deixar um valor pendente ao encerrar a operação antes do prazo.
func stopTimer(timer *time.Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

// NewSimulatedProcessor cria um Processor que apenas espera pelo tempo calculado.
// Ele modela uma operação com custo fixo por chamada e custo variável por item:
//
//	duração = fixedCost + perJobCost * quantidade de itens
//
// Como o custo fixo é pago uma vez por lote, comparar BatchSize 1 e 10 torna
// visível quando agrupar itens aumenta o throughput. O select respeita o
// cancelamento do contexto durante um shutdown forçado.
func NewSimulatedProcessor(fixedCost, perJobCost time.Duration) Processor {
	return func(ctx context.Context, batch []Job) error {
		delay := fixedCost + perJobCost*time.Duration(len(batch))
		timer := time.NewTimer(delay)
		defer stopTimer(timer)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		}
	}
}
