// Package kitchen contém a comanda, a fila limitada e o conjunto de
// cozinheiros que preparam os pratos aceitos pela API.
package kitchen

import "time"

// Ticket é a unidade de trabalho colocada na fila.
//
// ID identifica esta comanda dentro do processo. Dish é o prato pedido pelo
// cliente. EnqueuedAt registra quando a API aceitou o ticket.
type Ticket struct {
	ID         string
	Dish       string
	EnqueuedAt time.Time
}
