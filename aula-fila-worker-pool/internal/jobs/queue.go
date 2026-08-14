package jobs

import "errors"

// ErrQueueFull informa ao produtor que não havia espaço livre no instante da
// tentativa. O Handler usa esse erro para responder 503 sem ficar bloqueado.
var ErrQueueFull = errors.New("fila cheia")

// Queue é uma fila limitada e local ao processo. O channel guarda no máximo
// capacity tarefas; por isso a memória e a espera não crescem sem limite.
// Ela é apropriada para o laboratório, mas não sobrevive ao reinício do processo.
type Queue struct {
	jobs chan Job
}

// NewQueue cria o channel que armazena os trabalhos ainda não retirados pelos
// workers. capacity é a quantidade máxima de itens esperando no buffer; ela não
// inclui trabalhos que já estão sendo processados.
//
// Capacidade menor ou igual a zero indica erro de programação na montagem e
// causa panic antes de o servidor começar a aceitar requisições.
func NewQueue(capacity int) *Queue {
	if capacity <= 0 {
		panic("a capacidade da fila deve ser positiva")
	}
	return &Queue{jobs: make(chan Job, capacity)}
}

// TryEnqueue tenta colocar job no buffer sem esperar por espaço.
//
// Em um select, o caso default é executado quando nenhum outro caso pode
// prosseguir imediatamente. Portanto, se o channel estiver cheio, o método
// devolve ErrQueueFull e permite que a camada HTTP aplique rejeição controlada.
func (q *Queue) TryEnqueue(job Job) error {
	select {
	case q.jobs <- job:
		return nil
	default:
		return ErrQueueFull
	}
}

// Jobs oferece aos consumidores uma visão somente de leitura do channel. O tipo
// <-chan Job permite receber trabalhos, mas não permite enviar nem fechar a fila;
// essas operações continuam sob responsabilidade de Queue e do composition root,
// o ponto de montagem que coordena o encerramento da aplicação.
func (q *Queue) Jobs() <-chan Job {
	return q.jobs
}

// Depth devolve quantos trabalhos estão esperando no buffer naquele instante.
// O valor é uma fotografia: outro worker pode retirar um item logo depois da
// leitura. Trabalhos que já estão com workers não fazem parte desta contagem.
func (q *Queue) Depth() int {
	return len(q.jobs)
}

// Capacity devolve o limite fixo definido quando a fila foi criada.
func (q *Queue) Capacity() int {
	return cap(q.jobs)
}

// Close informa aos workers que nenhum trabalho novo será enviado. O main só
// chama Close depois de parar a entrada HTTP, evitando envio em channel fechado.
func (q *Queue) Close() {
	close(q.jobs)
}
