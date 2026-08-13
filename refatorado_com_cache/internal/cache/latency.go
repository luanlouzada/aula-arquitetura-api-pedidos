package cache

import (
	"math"
	"sort"
	"sync"
	"time"
)

const latencyWindowSize = 2048

// LatencyWindow guarda uma janela limitada de durações de hit e miss. Ela é
// suficiente para o laboratório mostrar p50 e p95 sem crescer indefinidamente.
// Em produção, histogramas de Prometheus substituiriam esta estrutura local.
type LatencyWindow struct {
	mu     sync.Mutex
	hits   []time.Duration
	misses []time.Duration
}

type LatencySnapshot struct {
	HitCount  int     `json:"hit_count"`
	HitP50MS  float64 `json:"hit_p50_ms"`
	HitP95MS  float64 `json:"hit_p95_ms"`
	MissCount int     `json:"miss_count"`
	MissP50MS float64 `json:"miss_p50_ms"`
	MissP95MS float64 `json:"miss_p95_ms"`
}

func (window *LatencyWindow) RecordHit(duration time.Duration) {
	window.mu.Lock()
	defer window.mu.Unlock()
	window.hits = appendLimited(window.hits, duration)
}

func (window *LatencyWindow) RecordMiss(duration time.Duration) {
	window.mu.Lock()
	defer window.mu.Unlock()
	window.misses = appendLimited(window.misses, duration)
}

func (window *LatencyWindow) Snapshot() LatencySnapshot {
	window.mu.Lock()
	hits := append([]time.Duration(nil), window.hits...)
	misses := append([]time.Duration(nil), window.misses...)
	window.mu.Unlock()
	return LatencySnapshot{
		HitCount:  len(hits),
		HitP50MS:  milliseconds(percentile(hits, 0.50)),
		HitP95MS:  milliseconds(percentile(hits, 0.95)),
		MissCount: len(misses),
		MissP50MS: milliseconds(percentile(misses, 0.50)),
		MissP95MS: milliseconds(percentile(misses, 0.95)),
	}
}

func milliseconds(duration time.Duration) float64 {
	return float64(duration) / float64(time.Millisecond)
}

func appendLimited(values []time.Duration, value time.Duration) []time.Duration {
	if len(values) == latencyWindowSize {
		copy(values, values[1:])
		values[len(values)-1] = value
		return values
	}
	return append(values, value)
}

func percentile(values []time.Duration, fraction float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	sort.Slice(values, func(left, right int) bool { return values[left] < values[right] })
	index := int(math.Ceil(float64(len(values))*fraction)) - 1
	if index < 0 {
		index = 0
	}
	return values[index]
}
