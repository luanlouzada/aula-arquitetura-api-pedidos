package application

import (
	"context"

	"aula-pedidos/refatorado_com_cache/internal/domain"
)

// Este arquivo declara somente a necessidade de persistência dos casos de uso;
// a implementação concreta está na infraestrutura.

// OrderRepository é o contrato de persistência declarado pela camada que o
// consome. Ele trabalha com Order inteiro porque o pedido é a raiz do agregado:
// as tabelas orders e order_items são um detalhe do PostgreSQL, não dois objetos
// que a aplicação deva salvar independentemente.
//
// A interface menciona intenção — criar, buscar e salvar — sem mencionar SQL,
// pgx ou nomes de tabelas. Uma implementação em memória ou outro banco pode
// satisfazer o mesmo contrato sem alterar OrderService.
type OrderRepository interface {
	// Create persiste um pedido novo e devolve o agregado com dados atribuídos
	// pela persistência, como ID e data de criação.
	Create(ctx context.Context, order domain.Order) (domain.Order, error)
	// FindByID carrega e reconstitui o agregado completo identificado por id.
	FindByID(ctx context.Context, id int64) (domain.Order, error)
	// Save persiste uma mudança em um pedido existente e devolve sua nova versão.
	Save(ctx context.Context, order domain.Order) (domain.Order, error)
}
