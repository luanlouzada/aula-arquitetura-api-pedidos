// Package lifecycle contém o algoritmo de encerramento a ser completado.
package lifecycle

import (
	"context"
	"errors"
	"time"
)

// Steps agrupa ações concretas sem dar ao package lifecycle conhecimento de
// http.Server, Queue ou Pool. O exercício deve coordenar, não recriar componentes.
type Steps struct {
	MarkNotReady     func()
	ShutdownHTTP     func(context.Context) error
	ForceCloseHTTP   func() error
	CloseQueue       func()
	WaitWorkers      func()
	ForceStopWorkers func()
}

// Shutdown deve coordenar as ações recebidas dentro do prazo de ctx. A ordem, o
// tratamento de falhas e o caminho de timeout devem ser deduzidos dos invariantes
// descritos no README e dos contratos executáveis nos testes.
func Shutdown(ctx context.Context, propagationDelay time.Duration, steps Steps) error {
	// TODO 8: implemente o encerramento sem abandonar trabalho nem esperar para sempre.
	return errors.New("TODO: implemente o shutdown gracioso")
}
