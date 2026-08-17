// Package lifecycle coordena a ordem do encerramento gracioso. As funções
// recebidas mantêm o algoritmo testável e deixam os componentes concretos no
// composition root.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Steps reúne ações que pertencem a componentes diferentes. Shutdown conhece a
// ordem entre elas, enquanto cmd/api fornece as funções concretas do servidor,
// da fila, do estado de saúde e do Pool.
type Steps struct {
	MarkNotReady     func()
	ShutdownHTTP     func(context.Context) error
	ForceCloseHTTP   func() error
	CloseQueue       func()
	WaitWorkers      func()
	ForceStopWorkers func()
}

// Shutdown executa uma ordem deliberada:
//
//  1. anuncia que a instância não aceita tráfego novo;
//  2. espera o balanceador propagar essa decisão;
//  3. encerra o HTTP e aguarda handlers em andamento;
//  4. fecha a produção da fila;
//  5. espera os workers drenarem o trabalho já aceito;
//  6. cancela os workers somente se o prazo terminar.
func Shutdown(ctx context.Context, propagationDelay time.Duration, steps Steps) error {
	if err := validate(steps); err != nil {
		return err
	}
	// main colocou state.MarkNotReady neste campo. A chamada muda o mesmo State
	// consultado por /readyz e POST /jobs; lifecycle não possui outro estado.
	steps.MarkNotReady()

	var shutdownErrors []error
	// Em uma infraestrutura que observa readiness, este intervalo dá tempo para a
	// retirada se propagar. No laboratório, o script também retira o upstream do
	// NGINX explicitamente antes de o Docker enviar SIGTERM.
	if err := wait(ctx, propagationDelay); err != nil {
		shutdownErrors = append(shutdownErrors, fmt.Errorf("propagar retirada: %w", err))
	}
	// main colocou http.Server.Shutdown neste campo. Ele fecha os listeners e
	// espera os handlers HTTP ativos terminarem dentro do prazo de ctx.
	if err := steps.ShutdownHTTP(ctx); err != nil {
		shutdownErrors = append(shutdownErrors, fmt.Errorf("encerrar HTTP: %w", err))
		if closeErr := steps.ForceCloseHTTP(); closeErr != nil {
			shutdownErrors = append(shutdownErrors, fmt.Errorf("forçar fechamento HTTP: %w", closeErr))
		}
	}

	// main colocou Queue.Close neste campo. No caminho gracioso, ShutdownHTTP já
	// esperou os handlers. Se o prazo exigiu ForceCloseHTTP, algum handler ainda
	// pode terminar em paralelo; a própria Queue sincroniza Close com TryEnqueue e
	// devolve ErrQueueClosed. Os Jobs que já estão no buffer não são apagados.
	steps.CloseQueue()
	drained := make(chan struct{})
	// Pool.Wait bloqueia até todos os workers saírem. Ele roda em outra goroutine
	// para que este coordenador possa observar simultaneamente o prazo de ctx.
	go func() {
		steps.WaitWorkers()
		close(drained)
	}()

	select {
	case <-drained:
	case <-ctx.Done():
		// main colocou cancelWorkers neste campo. O cancelamento chega ao Processor
		// pelo workerContext e libera trabalhos que não terminaram dentro do prazo.
		steps.ForceStopWorkers()
		// Mesmo no caminho forçado, não retornamos enquanto goroutines do Pool ainda
		// estiverem vivas dentro deste processo.
		<-drained
		shutdownErrors = append(shutdownErrors, fmt.Errorf("drenar fila: %w", ctx.Err()))
	}
	return errors.Join(shutdownErrors...)
}

// wait implementa o intervalo de propagação sem ignorar o prazo global. Usar
// time.Sleep aqui impediria uma saída imediata quando o contexto fosse cancelado.
func wait(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// validate falha antes da primeira mudança de estado. Descobrir uma função
// ausente depois de marcar not-ready deixaria o serviço parcialmente encerrado.
func validate(steps Steps) error {
	if steps.MarkNotReady == nil || steps.ShutdownHTTP == nil || steps.ForceCloseHTTP == nil ||
		steps.CloseQueue == nil || steps.WaitWorkers == nil || steps.ForceStopWorkers == nil {
		return errors.New("todas as etapas de shutdown são obrigatórias")
	}
	return nil
}
