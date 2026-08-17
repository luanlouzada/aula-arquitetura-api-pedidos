package exports

import (
	"errors"
	"sync"
)

// ErrQueueFull representa falta temporária de espaço, não um defeito inesperado.
// O Handler reconhece esse valor e o traduz para 503 com reason=queue_full.
var ErrQueueFull = errors.New("fila de exportações cheia")

// ErrQueueClosed representa uma tentativa que encontrou a entrada já encerrada
// pelo shutdown. A Queue trata essa corrida internamente porque sincronização do
// channel não faz parte das políticas deixadas como TODO.
var ErrQueueClosed = errors.New("fila de exportações fechada")

// Queue mantém somente exportações que ainda esperam por um worker. O channel
// com buffer oferece FIFO (primeiro a entrar, primeiro a sair) na retirada e um
// limite fixo de memória.
type Queue struct {
	mutex   sync.RWMutex
	exports chan Export
	closed  bool
}

// NewQueue cria o buffer de espera. Capacidade inválida é erro de montagem e
// causa panic antes de o servidor aceitar requisições.
func NewQueue(capacity int) *Queue {
	if capacity <= 0 {
		panic("a capacidade da fila deve ser positiva")
	}
	return &Queue{exports: make(chan Export, capacity)}
}

// TryEnqueue decide entre aceitar e devolver ErrQueueFull sem esperar uma vaga.
// Ele só pode aguardar brevemente a sincronização com Close; esperar capacidade
// moveria a fila para as conexões HTTP.
func (q *Queue) TryEnqueue(item Export) error {
	// Manter o lock até o fim do send não bloqueante impede que Close feche o
	// channel entre a verificação de estado e o envio.
	q.mutex.RLock()
	defer q.mutex.RUnlock()
	if q.closed {
		return ErrQueueClosed
	}
	select {
	case q.exports <- item:
		return nil
	default:
		return ErrQueueFull
	}
}

// Exports oferece aos workers uma visão somente de leitura do channel.
func (q *Queue) Exports() <-chan Export { return q.exports }

// Depth informa quantas exportações ainda esperam no buffer neste instante.
func (q *Queue) Depth() int { return len(q.exports) }

// Capacity informa o teto fixo do buffer, e não as vagas restantes.
func (q *Queue) Capacity() int { return cap(q.exports) }

// Close encerra a produção sem apagar itens aceitos. A operação é idempotente e
// sincronizada com TryEnqueue; workers drenam o buffer antes de encerrar.
func (q *Queue) Close() {
	q.mutex.Lock()
	defer q.mutex.Unlock()
	if q.closed {
		return
	}
	q.closed = true
	close(q.exports)
}
