package jobs

import (
	"errors"
	"sync"
)

// ErrQueueFull é um resultado esperado de capacidade, não uma falha interna. A
// camada HTTP o traduz para 503 e pode orientar o cliente com Retry-After.
var ErrQueueFull = errors.New("fila cheia")

// ErrQueueClosed informa que o ciclo de shutdown já encerrou a entrada. É um
// estado operacional esperado: um handler que já estava em andamento pode
// chegar à fila enquanto o fechamento forçado do HTTP ainda termina.
var ErrQueueClosed = errors.New("fila fechada")

// Queue usa um channel com buffer fixo. A capacidade limita somente itens em
// espera; trabalhos que um worker já retirou são contados como in-flight.
type Queue struct {
	mutex  sync.RWMutex
	jobs   chan Job
	closed bool
}

// NewQueue reserva um channel com número fixo de posições de espera. O panic em
// capacidade inválida ocorre durante a montagem, antes de aceitar tráfego.
func NewQueue(capacity int) *Queue {
	if capacity <= 0 {
		panic("a capacidade da fila deve ser positiva")
	}
	return &Queue{jobs: make(chan Job, capacity)}
}

// TryEnqueue nunca espera por uma vaga. Ele pode aguardar brevemente a
// sincronização com Close, mas não espera um worker liberar capacidade; essa
// espera transformaria a fila limitada em conexões HTTP acumuladas fora dela.
func (q *Queue) TryEnqueue(job Job) error {
	// O lock permanece durante a tentativa de envio para que Close nunca feche o
	// channel entre a verificação e o send. Como o select possui default, esta
	// região crítica não fica esperando por espaço na fila.
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	if q.closed {
		return ErrQueueClosed
	}
	select {
	case q.jobs <- job:
		return nil
	default:
		return ErrQueueFull
	}
}

// Jobs entrega ao Pool uma visão somente de leitura. O Pool pode consumir, mas
// não pode enviar nem fechar a fila que pertence ao fluxo de entrada.
func (q *Queue) Jobs() <-chan Job { return q.jobs }

// Depth é uma fotografia dos itens ainda no buffer. Um Job já retirado por um
// worker não está mais na fila; ele aparece como in-flight nas métricas.
func (q *Queue) Depth() int { return len(q.jobs) }

// Capacity devolve o teto fixado em NewQueue, não a quantidade de vagas livres.
func (q *Queue) Capacity() int { return cap(q.jobs) }

// Close informa que nenhum produtor enviará novos Jobs. O mutex coordena Close
// com handlers que já estavam em TryEnqueue; closed torna a operação idempotente.
// Os valores que estavam no buffer continuam disponíveis para os workers.
func (q *Queue) Close() {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if q.closed {
		return
	}
	q.closed = true
	close(q.jobs)
}
