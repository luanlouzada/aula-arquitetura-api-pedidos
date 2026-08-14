// Package telemetry guarda métricas pequenas para o laboratório. Em produção,
// os mesmos sinais normalmente seriam publicados para Prometheus ou outro
// backend e sobreviveriam ao processo que os produziu.
package telemetry

import (
	"sync/atomic"
	"time"
)

// Metrics reúne contadores atualizados simultaneamente pelos handlers HTTP e
// pelos workers. Os tipos atomic executam Add e Load como operações indivisíveis.
// Assim, duas goroutines não leem e sobrescrevem o mesmo contador ao mesmo tempo,
// situação conhecida como data race, e não é necessário proteger cada requisição
// com um mutex compartilhado.
//
// As métricas vivem somente na memória e voltam a zero quando a API reinicia.
// Como cada contador é lido separadamente, Snapshot é adequado para observação,
// mas não representa uma transação única entre todos os campos.
type Metrics struct {
	accepted         atomic.Uint64
	rejected         atomic.Uint64
	started          atomic.Uint64
	completed        atomic.Uint64
	failed           atomic.Uint64
	batches          atomic.Uint64
	batchItems       atomic.Uint64
	inFlight         atomic.Int64
	totalQueueWaitNS atomic.Uint64
	totalBatchTimeNS atomic.Uint64
}

// RecordAccepted conta um trabalho que entrou na fila. Aceito não significa
// concluído: o item ainda pode estar esperando ou sendo processado.
func (m *Metrics) RecordAccepted() {
	m.accepted.Add(1)
}

// RecordRejected conta uma tentativa recusada porque a fila estava cheia.
func (m *Metrics) RecordRejected() {
	m.rejected.Add(1)
}

// RecordBatchStart registra que items deixaram a espera e passaram para
// processamento. totalQueueWait é a soma da espera de todos os itens do lote;
// essa soma permite calcular depois a espera média por trabalho.
func (m *Metrics) RecordBatchStart(items int, totalQueueWait time.Duration) {
	m.started.Add(uint64(items))
	m.inFlight.Add(int64(items))
	m.totalQueueWaitNS.Add(uint64(totalQueueWait.Nanoseconds()))
}

// RecordBatchEnd encerra a medição iniciada por RecordBatchStart. elapsed mede o
// lote inteiro. Se Processor retornou erro, todos os itens do lote são contados
// como falhos; caso contrário, são contados como concluídos.
func (m *Metrics) RecordBatchEnd(items int, elapsed time.Duration, err error) {
	m.batches.Add(1)
	m.batchItems.Add(uint64(items))
	m.inFlight.Add(-int64(items))
	m.totalBatchTimeNS.Add(uint64(elapsed.Nanoseconds()))
	if err != nil {
		m.failed.Add(uint64(items))
		return
	}
	m.completed.Add(uint64(items))
}

// Snapshot é a representação JSON devolvida por GET /stats. Ela separa sinais
// de fila, workers, trabalhos e batching para que cada número tenha contexto.
type Snapshot struct {
	Queue struct {
		Depth       int     `json:"depth"`
		Capacity    int     `json:"capacity"`
		Utilization float64 `json:"utilization"`
	} `json:"queue"`
	Workers struct {
		Configured int   `json:"configured"`
		InFlight   int64 `json:"in_flight"`
	} `json:"workers"`
	Jobs struct {
		Accepted  int64 `json:"accepted"`
		Rejected  int64 `json:"rejected"`
		Started   int64 `json:"started"`
		Completed int64 `json:"completed"`
		Failed    int64 `json:"failed"`
	} `json:"jobs"`
	Batching struct {
		Batches                  int64   `json:"batches"`
		AverageBatchSize         float64 `json:"average_batch_size"`
		AverageQueueWaitMS       float64 `json:"average_queue_wait_ms"`
		AverageBatchProcessingMS float64 `json:"average_batch_processing_ms"`
	} `json:"batching"`
}

// Snapshot lê os contadores atuais e calcula proporções e médias derivadas.
// queueDepth e queueCapacity vêm da Queue, enquanto workers vem da configuração;
// recebê-los como parâmetros evita que o pacote de métricas conheça esses tipos.
//
// Divisões só são feitas quando o denominador é maior que zero. Antes do
// primeiro lote, as médias permanecem com o valor zero do tipo float64.
func (m *Metrics) Snapshot(queueDepth, queueCapacity, workers int) Snapshot {
	accepted := m.accepted.Load()
	rejected := m.rejected.Load()
	started := m.started.Load()
	completed := m.completed.Load()
	failed := m.failed.Load()
	batches := m.batches.Load()
	batchItems := m.batchItems.Load()

	var snapshot Snapshot
	snapshot.Queue.Depth = queueDepth
	snapshot.Queue.Capacity = queueCapacity
	if queueCapacity > 0 {
		snapshot.Queue.Utilization = float64(queueDepth) / float64(queueCapacity)
	}
	snapshot.Workers.Configured = workers
	snapshot.Workers.InFlight = m.inFlight.Load()
	snapshot.Jobs.Accepted = int64(accepted)
	snapshot.Jobs.Rejected = int64(rejected)
	snapshot.Jobs.Started = int64(started)
	snapshot.Jobs.Completed = int64(completed)
	snapshot.Jobs.Failed = int64(failed)
	snapshot.Batching.Batches = int64(batches)
	if batches > 0 {
		snapshot.Batching.AverageBatchSize = float64(batchItems) / float64(batches)
		snapshot.Batching.AverageBatchProcessingMS =
			float64(m.totalBatchTimeNS.Load()) / float64(batches) / float64(time.Millisecond)
	}
	if started > 0 {
		snapshot.Batching.AverageQueueWaitMS =
			float64(m.totalQueueWaitNS.Load()) / float64(started) / float64(time.Millisecond)
	}
	return snapshot
}
