package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"aula-pedidos/refatorado_com_cache/internal/application"
	"aula-pedidos/refatorado_com_cache/internal/domain"
)

// Estes testes exercitam os casos de uso com uma implementação em memória.
// OrderService depende do contrato e pode ser verificado sem iniciar PostgreSQL
// ou um servidor HTTP.

// TestOrderServiceLifecycleWithoutPostgreSQL percorre criação, pagamento e uma
// tentativa inválida de cancelamento, observando também a evolução da versão.
func TestOrderServiceLifecycleWithoutPostgreSQL(t *testing.T) {
	repository := newFakeOrderRepository()
	service := application.NewOrderService(repository)

	created, err := service.Create(context.Background(), application.CreateOrderInput{
		Customer: "Ana",
		Items: []application.CreateOrderItemInput{
			{ProductName: "Notebook", UnitPriceCents: 450_000, Quantity: 2},
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ID() == 0 {
		t.Fatal("Create() returned an order without ID")
	}
	if got, want := created.Status(), domain.StatusPending; got != want {
		t.Fatalf("created Status() = %q, want %q", got, want)
	}

	paid, err := service.Pay(context.Background(), created.ID())
	if err != nil {
		t.Fatalf("Pay() error = %v", err)
	}
	if got, want := paid.Status(), domain.StatusPaid; got != want {
		t.Fatalf("paid Status() = %q, want %q", got, want)
	}
	if got, want := paid.Version(), 2; got != want {
		t.Fatalf("paid Version() = %d, want %d", got, want)
	}

	_, err = service.Cancel(context.Background(), created.ID())
	if !errors.Is(err, domain.ErrInvalidStatusTransition) {
		t.Fatalf("Cancel() error = %v, want %v", err, domain.ErrInvalidStatusTransition)
	}
}

// fakeOrderRepository é uma implementação de teste do mesmo contrato usado pelo
// adaptador PostgreSQL. Ele não é um mock do pgx: substitui toda a fronteira de
// persistência pela necessidade que a aplicação realmente declarou.
type fakeOrderRepository struct {
	nextID int64
	orders map[int64]domain.Order
}

// newFakeOrderRepository inicializa armazenamento e sequência de IDs isolados
// para cada teste.
func newFakeOrderRepository() *fakeOrderRepository {
	return &fakeOrderRepository{
		nextID: 1,
		orders: make(map[int64]domain.Order),
	}
}

// Create simula os valores que normalmente seriam atribuídos pelo banco e usa
// RestoreOrder para devolver um agregado persistido válido.
func (repository *fakeOrderRepository) Create(_ context.Context, order domain.Order) (domain.Order, error) {
	id := repository.nextID
	repository.nextID++
	persisted, err := domain.RestoreOrder(
		id,
		order.Customer(),
		order.Status(),
		order.TotalCents(),
		1,
		time.Date(2026, time.August, 4, 12, 0, 0, 0, time.UTC),
		order.Items(),
	)
	if err != nil {
		return domain.Order{}, err
	}
	repository.orders[id] = persisted
	return persisted, nil
}

// FindByID simula a leitura pelo identificador e preserva o mesmo erro estável
// esperado pelos casos de uso quando o pedido não existe.
func (repository *fakeOrderRepository) FindByID(_ context.Context, id int64) (domain.Order, error) {
	order, found := repository.orders[id]
	if !found {
		return domain.Order{}, domain.ErrOrderNotFound
	}
	return order, nil
}

// Save simula a concorrência otimista: só aceita a gravação se a versão recebida
// ainda for a versão armazenada e incrementa a versão no retorno.
func (repository *fakeOrderRepository) Save(_ context.Context, order domain.Order) (domain.Order, error) {
	stored, found := repository.orders[order.ID()]
	if !found {
		return domain.Order{}, domain.ErrOrderNotFound
	}
	if stored.Version() != order.Version() {
		return domain.Order{}, domain.ErrConcurrentModification
	}
	persisted, err := domain.RestoreOrder(
		order.ID(),
		order.Customer(),
		order.Status(),
		order.TotalCents(),
		order.Version()+1,
		order.CreatedAt(),
		order.Items(),
	)
	if err != nil {
		return domain.Order{}, err
	}
	repository.orders[order.ID()] = persisted
	return persisted, nil
}

// Assim como na implementação PostgreSQL, esta linha faz o compilador verificar
// que o fake continua satisfazendo o contrato usado por OrderService.
var _ application.OrderRepository = (*fakeOrderRepository)(nil)
