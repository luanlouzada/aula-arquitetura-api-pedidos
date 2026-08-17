// Package jobs contém o trabalho assíncrono, a fila limitada e o pool fixo de
// workers. Esses mecanismos já foram estudados no laboratório anterior; aqui
// eles formam a capacidade protegida pelo limitador.
package jobs

import "time"

// Job é o valor aceito pelo HTTP e entregue a exatamente um worker. EnqueuedAt
// permite medir quanto tempo o trabalho ficou esperando antes de começar.
type Job struct {
	ID         string
	OrderID    int64
	EnqueuedAt time.Time
}
