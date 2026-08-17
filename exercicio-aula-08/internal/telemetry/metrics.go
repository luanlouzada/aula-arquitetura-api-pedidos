// Package telemetry torna entrada, espera e conclusão observáveis dentro de uma
// instância. Os contadores são locais e reiniciam junto com o processo.
package telemetry

import (
	"sync/atomic"
	"time"
)

// Metrics é compartilhado por handlers e workers. Tipos atomic permitem que
// várias goroutines atualizem contadores sem data race e sem um mutex global.
type Metrics struct {
	instanceID       string
	startedAt        time.Time
	requests         atomic.Uint64
	invalid          atomic.Uint64
	accepted         atomic.Uint64
	rateLimited      atomic.Uint64
	queueFull        atomic.Uint64
	notReady         atomic.Uint64
	started          atomic.Uint64
	completed        atomic.Uint64
	failed           atomic.Uint64
	inFlight         atomic.Int64
	totalQueueWaitNS atomic.Uint64
}

// New fixa a identidade e o início da janela usada no goodput médio.
func New(instanceID string) *Metrics {
	return &Metrics{instanceID: instanceID, startedAt: time.Now()}
}

// RecordRequest conta toda tentativa recebida por POST /exports.
func (m *Metrics) RecordRequest() { m.requests.Add(1) }

// RecordInvalid conta solicitações encerradas com 400 antes da admissão.
func (m *Metrics) RecordInvalid() { m.invalid.Add(1) }

// RecordAccepted conta 202: a exportação entrou na fila, mas não terminou ainda.
func (m *Metrics) RecordAccepted() { m.accepted.Add(1) }

// RecordRateLimited conta rejeições 429 produzidas pelo token bucket.
func (m *Metrics) RecordRateLimited() { m.rateLimited.Add(1) }

// RecordQueueFull conta rejeições 503 causadas por falta de vaga no buffer.
func (m *Metrics) RecordQueueFull() { m.queueFull.Add(1) }

// RecordNotReady conta operações recusadas enquanto a instância está em drain.
func (m *Metrics) RecordNotReady() { m.notReady.Add(1) }

// RecordExportStarted move uma exportação da espera para in-flight e acumula o
// tempo passado na fila para permitir o cálculo posterior da média.
func (m *Metrics) RecordExportStarted(wait time.Duration) {
	m.started.Add(1)
	m.inFlight.Add(1)
	m.totalQueueWaitNS.Add(uint64(wait.Nanoseconds()))
}

// RecordExportFinished classifica o resultado. Goodput considera completed e
// exclui falhas, pois mede somente trabalho útil terminado com sucesso.
func (m *Metrics) RecordExportFinished(err error) {
	m.inFlight.Add(-1)
	if err != nil {
		m.failed.Add(1)
		return
	}
	m.completed.Add(1)
}

// Runtime contém a fotografia de valores que não são contadores: fila atual,
// limites configurados, prontidão e saldo aproximado do token bucket.
type Runtime struct {
	Ready         bool
	QueueDepth    int
	QueueCapacity int
	Workers       int
	RatePerSecond float64
	Burst         int
	Tokens        float64
}

// Snapshot é o contrato JSON de GET /stats. Separar requests de exports impede
// confundir uma resposta 202 com uma exportação já concluída.
type Snapshot struct {
	InstanceID    string  `json:"instance_id"`
	UptimeSeconds float64 `json:"uptime_seconds"`
	Ready         bool    `json:"ready"`
	TokenBucket   struct {
		RatePerSecond float64 `json:"rate_per_second"`
		Burst         int     `json:"burst"`
		TokensNow     float64 `json:"tokens_now"`
	} `json:"token_bucket"`
	Requests struct {
		Total          uint64 `json:"total"`
		Invalid400     uint64 `json:"invalid_400"`
		Accepted202    uint64 `json:"accepted_202"`
		RateLimited429 uint64 `json:"rate_limited_429"`
		QueueFull503   uint64 `json:"queue_full_503"`
		NotReady503    uint64 `json:"not_ready_503"`
	} `json:"requests"`
	Queue struct {
		Depth       int     `json:"depth"`
		Capacity    int     `json:"capacity"`
		Utilization float64 `json:"utilization"`
	} `json:"queue"`
	Workers struct {
		Configured int   `json:"configured"`
		InFlight   int64 `json:"in_flight"`
	} `json:"workers"`
	Exports struct {
		Started            uint64  `json:"started"`
		Completed          uint64  `json:"completed"`
		Failed             uint64  `json:"failed"`
		AverageQueueWaitMS float64 `json:"average_queue_wait_ms"`
		GoodputPerSecond   float64 `json:"goodput_exports_per_second"`
	} `json:"exports"`
}

// Snapshot combina leituras atômicas e calcula utilização, espera média e
// goodput. O conjunto é observacional: pode mudar enquanto o JSON é enviado.
func (m *Metrics) Snapshot(runtime Runtime) Snapshot {
	uptime := time.Since(m.startedAt)
	started := m.started.Load()
	completed := m.completed.Load()
	var snapshot Snapshot
	snapshot.InstanceID = m.instanceID
	snapshot.UptimeSeconds = uptime.Seconds()
	snapshot.Ready = runtime.Ready
	snapshot.TokenBucket.RatePerSecond = runtime.RatePerSecond
	snapshot.TokenBucket.Burst = runtime.Burst
	snapshot.TokenBucket.TokensNow = runtime.Tokens
	snapshot.Requests.Total = m.requests.Load()
	snapshot.Requests.Invalid400 = m.invalid.Load()
	snapshot.Requests.Accepted202 = m.accepted.Load()
	snapshot.Requests.RateLimited429 = m.rateLimited.Load()
	snapshot.Requests.QueueFull503 = m.queueFull.Load()
	snapshot.Requests.NotReady503 = m.notReady.Load()
	snapshot.Queue.Depth = runtime.QueueDepth
	snapshot.Queue.Capacity = runtime.QueueCapacity
	if runtime.QueueCapacity > 0 {
		snapshot.Queue.Utilization = float64(runtime.QueueDepth) / float64(runtime.QueueCapacity)
	}
	snapshot.Workers.Configured = runtime.Workers
	snapshot.Workers.InFlight = m.inFlight.Load()
	snapshot.Exports.Started = started
	snapshot.Exports.Completed = completed
	snapshot.Exports.Failed = m.failed.Load()
	if started > 0 {
		snapshot.Exports.AverageQueueWaitMS = float64(m.totalQueueWaitNS.Load()) / float64(started) / float64(time.Millisecond)
	}
	if uptime > 0 {
		snapshot.Exports.GoodputPerSecond = float64(completed) / uptime.Seconds()
	}
	return snapshot
}
