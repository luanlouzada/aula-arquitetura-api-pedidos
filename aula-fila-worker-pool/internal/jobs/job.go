// Package jobs contém a unidade de trabalho, a fila limitada e o conjunto de
// workers que processa os itens aceitos pela API. A fila deste laboratório vive
// somente na memória do processo; encerrar a aplicação descarta o que não foi
// concluído.
package jobs

import "time"

// Job é a unidade de trabalho colocada na fila.
//
// ID identifica esta tentativa de processamento dentro do processo. OrderID
// aponta para o pedido que originou o trabalho. EnqueuedAt registra quando a API
// aceitou o item e permite calcular quanto tempo ele aguardou até um worker
// começar a processá-lo.
type Job struct {
	ID         string
	OrderID    int64
	EnqueuedAt time.Time
}
