// Package telemetry torna as decisões de admissão e o trabalho útil
// observáveis sem exigir uma plataforma externa de métricas no laboratório.
package telemetry

import (
	"sync/atomic"
	"time"
)

// Metrics guarda contadores locais atualizados por handlers e workers ao mesmo
// tempo. Os tipos atomic evitam data race sem colocar todas as requisições atrás
// de um único mutex. Os valores voltam a zero quando o processo reinicia.
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

// New registra a identidade e o instante usados para calcular a média de
// goodput. Em produção, a taxa normalmente seria calculada fora do processo.
func New(instanceID string) *Metrics {
	return &Metrics{instanceID: instanceID, startedAt: time.Now()}
}

// RecordRequest conta toda tentativa que chegou a POST /jobs.
func (m *Metrics) RecordRequest() { m.requests.Add(1) }

// RecordInvalid conta entradas rejeitadas com 400 antes de consumir permissão.
func (m *Metrics) RecordInvalid() { m.invalid.Add(1) }

// RecordAccepted conta respostas 202, isto é, Jobs que entraram na fila. Ele
// não significa conclusão; essa diferença é a razão de existir completed.
func (m *Metrics) RecordAccepted() { m.accepted.Add(1) }

// RecordRateLimited conta respostas 429 produzidas antes de tocar na fila.
func (m *Metrics) RecordRateLimited() { m.rateLimited.Add(1) }

// RecordQueueFull conta respostas 503 causadas pelo buffer sem vaga.
func (m *Metrics) RecordQueueFull() { m.queueFull.Add(1) }

// RecordNotReady conta novas operações recusadas enquanto a instância drena.
func (m *Metrics) RecordNotReady() { m.notReady.Add(1) }

// RecordJobStarted move conceitualmente um Job da espera para in-flight e soma
// sua espera. Somar durações permite calcular a média sem guardar cada amostra.
func (m *Metrics) RecordJobStarted(wait time.Duration) {
	m.started.Add(1)
	m.inFlight.Add(1)
	m.totalQueueWaitNS.Add(uint64(wait.Nanoseconds()))
}

// RecordJobFinished retira um Job de in-flight e o classifica como concluído ou
// falho. Somente conclusões bem-sucedidas entram no goodput.
func (m *Metrics) RecordJobFinished(err error) {
	m.inFlight.Add(-1)
	if err != nil {
		m.failed.Add(1)
		return
	}
	m.completed.Add(1)
}

// Runtime contém valores que não pertencem aos contadores: estado atual da
// fila, configuração e saldo do limitador. Handler os coleta no mesmo instante.
type Runtime struct {
	Ready         bool
	QueueDepth    int
	QueueCapacity int
	Workers       int
	RatePerSecond float64
	Burst         int
	Tokens        float64
}

// Snapshot separa entrada, espera e processamento. Goodput usa somente Jobs
// concluídos com sucesso; requisições recebidas e respostas 202 não são trabalho
// útil concluído.
type Snapshot struct {
	InstanceID    string  `json:"instance_id"`
	UptimeSeconds float64 `json:"uptime_seconds"`
	Ready         bool    `json:"ready"`
	Admission     struct {
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
	Jobs struct {
		Started              uint64  `json:"started"`
		Completed            uint64  `json:"completed"`
		Failed               uint64  `json:"failed"`
		AverageQueueWaitMS   float64 `json:"average_queue_wait_ms"`
		GoodputJobsPerSecond float64 `json:"goodput_jobs_per_second"`
	} `json:"jobs"`
}

// Snapshot combina contadores monotônicos com a fotografia do Runtime e calcula
// proporções. Cada leitura atômica é segura, mas o conjunto não é uma transação:
// outra goroutine pode avançar um contador enquanto o JSON está sendo montado.
func (m *Metrics) Snapshot(runtime Runtime) Snapshot {
	uptime := time.Since(m.startedAt)
	started := m.started.Load()
	completed := m.completed.Load()

	var snapshot Snapshot
	snapshot.InstanceID = m.instanceID
	snapshot.UptimeSeconds = uptime.Seconds()
	snapshot.Ready = runtime.Ready
	snapshot.Admission.RatePerSecond = runtime.RatePerSecond
	snapshot.Admission.Burst = runtime.Burst
	snapshot.Admission.TokensNow = runtime.Tokens
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
	snapshot.Jobs.Started = started
	snapshot.Jobs.Completed = completed
	snapshot.Jobs.Failed = m.failed.Load()
	if started > 0 {
		snapshot.Jobs.AverageQueueWaitMS = float64(m.totalQueueWaitNS.Load()) / float64(started) / float64(time.Millisecond)
	}
	if uptime > 0 {
		snapshot.Jobs.GoodputJobsPerSecond = float64(completed) / uptime.Seconds()
	}
	return snapshot
}
