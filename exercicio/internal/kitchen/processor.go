package kitchen

import (
	"context"
	"time"
)

// Processor é o trabalho executado fora do mecanismo da fila: preparar um
// prato. Os testes podem substituir esta função por uma versão que apenas
// registra os tickets recebidos.
type Processor func(ctx context.Context, ticket Ticket) error

// Cook cria um Processor que apenas espera. O atraso modela o tempo de
// preparo e torna a capacidade da cozinha observável sob carga.
//
//	duração = fixedCost + perTicketCost
func Cook(fixedCost, perTicketCost time.Duration) Processor {
	return func(ctx context.Context, _ Ticket) error {
		delay := fixedCost + perTicketCost
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		}
	}
}
