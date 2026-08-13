package cache

import "sync/atomic"

// Metrics mantém contadores simples dentro deste processo. Eles tornam hit,
// miss e falhas observáveis sem introduzir Prometheus antes de o conceito de
// cache estar claro. Em várias réplicas, cada processo terá seus próprios dados.
type Metrics struct {
	hits   atomic.Uint64
	misses atomic.Uint64
	errors atomic.Uint64
}

// Snapshot é uma cópia consistente o bastante para observabilidade; os
// contadores podem continuar mudando enquanto a resposta é produzida.
type Snapshot struct {
	Hits     uint64  `json:"hits"`
	Misses   uint64  `json:"misses"`
	Errors   uint64  `json:"errors"`
	HitRatio float64 `json:"hit_ratio"`
}

func (metrics *Metrics) RecordHit()   { metrics.hits.Add(1) }
func (metrics *Metrics) RecordMiss()  { metrics.misses.Add(1) }
func (metrics *Metrics) RecordError() { metrics.errors.Add(1) }

// Snapshot calcula hits/(hits+misses). Erros ficam fora do denominador porque
// uma falha do Redis não confirma nem hit nem miss; a leitura usa o PostgreSQL.
func (metrics *Metrics) Snapshot() Snapshot {
	hits := metrics.hits.Load()
	misses := metrics.misses.Load()
	requests := hits + misses
	ratio := 0.0
	if requests > 0 {
		ratio = float64(hits) / float64(requests)
	}
	return Snapshot{
		Hits:     hits,
		Misses:   misses,
		Errors:   metrics.errors.Load(),
		HitRatio: ratio,
	}
}
