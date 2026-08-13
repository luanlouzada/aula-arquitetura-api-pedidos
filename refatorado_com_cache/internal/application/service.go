package application

import (
	"context"

	"aula-pedidos/refatorado_com_cache/internal/domain"
)

// Este arquivo contém os dados de entrada e a coordenação dos quatro casos de
// uso. Ele não contém DTOs JSON, códigos HTTP nem comandos SQL.

// CreateOrderInput representa o que o caso de uso precisa para criar um pedido.
// Não há tags json porque este formato pertence à aplicação, não ao protocolo HTTP.
type CreateOrderInput struct {
	Customer string
	Items    []CreateOrderItemInput
}

// CreateOrderItemInput representa um item recebido pelo caso de uso. Ainda não
// é um domain.Item: o construtor do domínio precisa validar seus valores.
type CreateOrderItemInput struct {
	ProductName    string
	UnitPriceCents int64
	Quantity       int
}

// OrderService coordena os casos de uso de pedido. Ele determina a sequência
// carregar → executar regra → salvar, enquanto as invariantes permanecem na
// entidade e os detalhes de persistência permanecem atrás de OrderRepository.
type OrderService struct {
	repository OrderRepository
}

// NewOrderService recebe a dependência de persistência pronta. Essa injeção de
// dependência permite usar PostgreSQL em produção e um fake rápido nos testes,
// sem colocar condições ou imports de infraestrutura dentro do service.
func NewOrderService(repository OrderRepository) *OrderService {
	return &OrderService{repository: repository}
}

// Create executa o caso de uso de criação. Ele converte a entrada da aplicação
// em objetos do domínio, deixa NewItem e NewOrder protegerem as invariantes e
// só então solicita que o agregado válido seja persistido.
func (service *OrderService) Create(ctx context.Context, input CreateOrderInput) (domain.Order, error) {
	items := make([]domain.Item, 0, len(input.Items))
	for _, inputItem := range input.Items {
		// Não montamos Item com literal de struct: NewItem é o ponto de criação
		// que garante produto, preço, quantidade e subtotal válidos.
		item, err := domain.NewItem(inputItem.ProductName, inputItem.UnitPriceCents, inputItem.Quantity)
		if err != nil {
			return domain.Order{}, err
		}
		items = append(items, item)
	}

	order, err := domain.NewOrder(input.Customer, items)
	if err != nil {
		return domain.Order{}, err
	}
	return service.repository.Create(ctx, order)
}

// Get executa uma consulta sem alterar o agregado. O service delega a busca ao
// contrato porque não precisa saber se os dados vêm de PostgreSQL ou memória.
func (service *OrderService) Get(ctx context.Context, id int64) (domain.Order, error) {
	return service.repository.FindByID(ctx, id)
}

// Pay executa o caso de uso de pagamento: carrega o agregado, pede que ele
// realize a transição e salva somente se a regra do domínio tiver permitido.
func (service *OrderService) Pay(ctx context.Context, id int64) (domain.Order, error) {
	order, err := service.repository.FindByID(ctx, id)
	if err != nil {
		return domain.Order{}, err
	}
	if err := order.Pay(); err != nil {
		return domain.Order{}, err
	}
	return service.repository.Save(ctx, order)
}

// Cancel segue o mesmo fluxo de Pay, mas solicita a transição para CANCELADO.
// O service não compara strings de status; Order.Cancel possui essa autoridade.
func (service *OrderService) Cancel(ctx context.Context, id int64) (domain.Order, error) {
	order, err := service.repository.FindByID(ctx, id)
	if err != nil {
		return domain.Order{}, err
	}
	if err := order.Cancel(); err != nil {
		return domain.Order{}, err
	}
	return service.repository.Save(ctx, order)
}
