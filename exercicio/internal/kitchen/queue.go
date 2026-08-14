package kitchen

import "errors"

// ErrQueueFull informa ao caixa que a fila não tinha espaço no
// instante da tentativa. O Handler usa esse erro para responder 503.
var ErrQueueFull = errors.New("fila cheia")

// Queue é a fila de espera da cozinha. Ela vive somente neste processo e tem
// capacidade fixa; quando não há vaga, novas comandas são recusadas.
//
// Preencha os campos necessários para guardar os tickets ainda não retirados
// pelos cozinheiros.
type Queue struct {
}

// NewQueue cria a fila com o número máximo de comandas que podem esperar.
// Capacidade menor ou igual a zero é erro de montagem.
func NewQueue(capacity int) *Queue {
	panic("TODO: implemente NewQueue")
}

// TryEnqueue tenta colocar o ticket na fila sem esperar por espaço.
// Se não houver vaga naquele instante, devolva ErrQueueFull.
func (q *Queue) TryEnqueue(ticket Ticket) error {
	panic("TODO: implemente TryEnqueue")
}

// Tickets oferece aos cozinheiros uma visão somente de leitura da fila.
// O Pool pode receber tickets, mas somente o Handler envia e somente cmd/api
// fecha a Queue durante o encerramento.
func (q *Queue) Tickets() <-chan Ticket {
	panic("TODO: implemente Tickets")
}

// Depth devolve quantos tickets estão esperando na fila naquele
// instante. É uma fotografia: um cozinheiro pode retirar um item logo depois.
func (q *Queue) Depth() int {
	panic("TODO: implemente Depth")
}

// Capacity devolve o limite fixo definido quando a fila foi criada.
func (q *Queue) Capacity() int {
	panic("TODO: implemente Capacity")
}

// Close informa aos cozinheiros que nenhuma comanda nova será enviada.
func (q *Queue) Close() {
	panic("TODO: implemente Close")
}
