package logging

import (
	"context"
	"log/slog"

	"aula-pedidos/refatorado_com_slog/internal/application"
	"aula-pedidos/refatorado_com_slog/internal/domain"
)

// LoggedOrderService é um decorator: oferece as mesmas operações usadas pelo
// Controller, delega o trabalho ao OrderService real e acrescenta logs depois
// que uma mudança de negócio termina com sucesso.
type LoggedOrderService struct {
	next   *application.OrderService
	logger *slog.Logger
}

// NewLoggedOrderService recebe objetos prontos. Ele não cria dependências nem
// lê configuração; essa montagem pertence ao cmd/api.
func NewLoggedOrderService(next *application.OrderService, logger *slog.Logger) *LoggedOrderService {
	return &LoggedOrderService{next: next, logger: logger}
}

// Create delega toda a regra ao caso de uso. O log só é emitido depois da
// persistência, evitando afirmar que um pedido foi criado quando houve erro.
func (logged *LoggedOrderService) Create(
	ctx context.Context,
	input application.CreateOrderInput,
) (domain.Order, error) {
	order, err := logged.next.Create(ctx, input)
	if err != nil {
		return domain.Order{}, err
	}
	logged.logger.InfoContext(
		ctx,
		"pedido criado",
		slog.String("operation", "orders.create"),
		slog.Int64("order_id", order.ID()),
		slog.String("order_status", string(order.Status())),
		slog.Int64("total_cents", order.TotalCents()),
	)
	return order, nil
}

// Get apenas delega a consulta. O middleware HTTP já registra que a requisição
// terminou; repetir um log de sucesso aqui acrescentaria ruído sem mudança de
// estado importante para registrar.
func (logged *LoggedOrderService) Get(ctx context.Context, id int64) (domain.Order, error) {
	return logged.next.Get(ctx, id)
}

// Pay registra o evento somente depois que o domínio permitiu a transição e o
// Repository salvou a nova versão do pedido.
func (logged *LoggedOrderService) Pay(ctx context.Context, id int64) (domain.Order, error) {
	order, err := logged.next.Pay(ctx, id)
	if err != nil {
		return domain.Order{}, err
	}
	logged.logger.InfoContext(
		ctx,
		"pedido pago",
		slog.String("operation", "orders.pay"),
		slog.Int64("order_id", order.ID()),
		slog.String("order_status", string(order.Status())),
		slog.Int("version", order.Version()),
	)
	return order, nil
}

// Cancel segue a mesma política de Pay: o registro descreve uma alteração que
// realmente foi persistida, não apenas uma tentativa de cancelamento.
func (logged *LoggedOrderService) Cancel(ctx context.Context, id int64) (domain.Order, error) {
	order, err := logged.next.Cancel(ctx, id)
	if err != nil {
		return domain.Order{}, err
	}
	logged.logger.InfoContext(
		ctx,
		"pedido cancelado",
		slog.String("operation", "orders.cancel"),
		slog.Int64("order_id", order.ID()),
		slog.String("order_status", string(order.Status())),
		slog.Int("version", order.Version()),
	)
	return order, nil
}
