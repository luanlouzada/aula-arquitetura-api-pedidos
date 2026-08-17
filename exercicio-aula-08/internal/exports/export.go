// Package exports contém a solicitação de exportação, a fila em memória e o
// pool fixo de geradores. Fila e workers são a base pronta deste exercício.
package exports

import "time"

// Export é a unidade de trabalho que recebeu 202. EnqueuedAt é definido pela
// API para medir a espera; o cliente escolhe somente qual relatório deseja.
type Export struct {
	ID         string
	Report     string
	EnqueuedAt time.Time
}
