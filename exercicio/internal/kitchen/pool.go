package kitchen

import (
	"context"
	"errors"
	"sync"
)

// Pool mantém uma quantidade fixa de cozinheiros consumindo a mesma fila.
//
// tickets é somente de leitura: o pool consome, mas não fecha o channel.
// Use wg em Start para que Wait consiga esperar o encerramento.
type Pool struct {
	tickets   <-chan Ticket
	cooks     int
	processor Processor
	wg        sync.WaitGroup
}

// NewPool valida e reúne as dependências. Ele ainda não inicia goroutines;
// Start realiza essa etapa depois que a aplicação foi montada.
func NewPool(tickets <-chan Ticket, cooks int, processor Processor) (*Pool, error) {
	if tickets == nil || processor == nil {
		return nil, errors.New("fila e processor são obrigatórios")
	}
	if cooks <= 0 {
		return nil, errors.New("cooks deve ser positivo")
	}
	return &Pool{
		tickets:   tickets,
		cooks:     cooks,
		processor: processor,
	}, nil
}

// Start cria uma quantidade fixa de goroutines consumidoras. Todas disputam
// a mesma fila e cada ticket é entregue a apenas um cozinheiro.
//
// Cada goroutine deve:
//   - sair quando o contexto for cancelado ou quando a fila for fechada;
//   - chamar p.processor para cada ticket recebido;
//   - usar p.wg (Add antes de iniciar, Done ao terminar) para Wait funcionar.
//
// Neste exercício cada cozinheiro prepara um ticket por vez. Não implemente
// agrupamento em lotes.
func (p *Pool) Start(ctx context.Context) {
	panic("TODO: implemente Start")
}

// Wait bloqueia até que todos os cozinheiros tenham saído.
func (p *Pool) Wait() {
	p.wg.Wait()
}
