package domain

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Este arquivo modela o agregado de pedido. Os campos ficam privados para que
// código externo não consiga montar estados inválidos com literais de struct ou
// alterar status, total e itens sem passar pelas operações do domínio.

// Status é um tipo do domínio, em vez de uma string genérica, para deixar
// explícito quais valores representam o ciclo de vida de um pedido.
type Status string

// Estes são os únicos estados aceitos pelo modelo. PAGO e CANCELADO são estados
// finais neste recorte: somente um pedido PENDENTE pode mudar de estado.
const (
	StatusPending  Status = "PENDENTE"
	StatusPaid     Status = "PAGO"
	StatusCanceled Status = "CANCELADO"
)

// Item é um objeto de valor interno do agregado Order. O negócio o reconhece
// por produto, preço e quantidade, não por uma identidade global própria. Seus
// campos privados garantem que todo Item tenha sido criado por NewItem.
type Item struct {
	productName    string
	unitPriceCents int64
	quantity       int
}

// NewItem cria um Item válido ou devolve um erro semântico. Além de exigir
// produto, preço e quantidade válidos, antecipa um possível overflow no
// subtotal. Assim, depois da construção, SubtotalCents pode multiplicar com
// segurança sem repetir as mesmas validações.
func NewItem(productName string, unitPriceCents int64, quantity int) (Item, error) {
	// Espaços nas extremidades não transformam um nome vazio em nome válido.
	productName = strings.TrimSpace(productName)
	if productName == "" || unitPriceCents <= 0 || quantity <= 0 {
		return Item{}, ErrInvalidItem
	}
	// A divisão testa o limite antes da multiplicação, pois multiplicar primeiro
	// poderia estourar int64 e produzir um subtotal incorreto.
	if unitPriceCents > math.MaxInt64/int64(quantity) {
		return Item{}, ErrOrderTotalOverflow
	}
	return Item{
		productName:    productName,
		unitPriceCents: unitPriceCents,
		quantity:       quantity,
	}, nil
}

// ProductName expõe o nome normalizado sem permitir que o chamador o altere.
func (item Item) ProductName() string {
	return item.productName
}

// UnitPriceCents devolve o preço em centavos. Usar inteiro evita erros de
// arredondamento de ponto flutuante em valores monetários.
func (item Item) UnitPriceCents() int64 {
	return item.unitPriceCents
}

// Quantity devolve a quantidade positiva protegida por NewItem.
func (item Item) Quantity() int {
	return item.quantity
}

// SubtotalCents calcula preço unitário × quantidade. A multiplicação é segura
// porque NewItem já rejeitou valores que excederiam o limite de int64.
func (item Item) SubtotalCents() int64 {
	return item.unitPriceCents * int64(item.quantity)
}

// Order é a raiz do agregado. Qualquer mudança que precise manter cliente,
// itens, total e status consistentes deve passar por suas operações. Os campos
// privados impedem, por exemplo, fazer order.status = StatusPaid fora do domínio.
type Order struct {
	id         int64
	customer   string
	status     Status
	totalCents int64
	version    int
	createdAt  time.Time
	items      []Item
}

// NewOrder cria um pedido novo. O domínio define PENDENTE como estado inicial,
// versão 1 e calcula o total a partir dos itens; por isso status, versão e total
// não são parâmetros controlados pelo cliente HTTP. ID e data serão atribuídos
// pela persistência e incorporados ao objeto retornado pelo Repository.
func NewOrder(customer string, items []Item) (Order, error) {
	// Normalizar antes de validar evita aceitar um cliente composto só por espaços.
	customer = strings.TrimSpace(customer)
	if customer == "" {
		return Order{}, ErrCustomerRequired
	}
	if len(items) == 0 {
		return Order{}, ErrOrderWithoutItems
	}

	// O total é um valor derivado: a raiz o calcula para não correr o risco de
	// receber um total incompatível com os itens.
	totalCents, err := calculateTotal(items)
	if err != nil {
		return Order{}, err
	}

	return Order{
		customer:   customer,
		status:     StatusPending,
		totalCents: totalCents,
		version:    1,
		// A cópia defensiva impede que o slice recebido seja alterado por fora e
		// mude silenciosamente o conteúdo do agregado.
		items: copyItems(items),
	}, nil
}

// RestoreOrder reconstitui um agregado que já existe na persistência.
// Diferentemente de NewOrder, ele recebe identidade, versão, data e estado
// anteriores. Isso não é um atalho para ignorar invariantes: dados técnicos,
// cliente, itens e total são validados antes de o objeto voltar à aplicação.
func RestoreOrder(
	id int64,
	customer string,
	status Status,
	totalCents int64,
	version int,
	createdAt time.Time,
	items []Item,
) (Order, error) {
	// Estes dados só existem depois da persistência e precisam formar uma
	// identidade válida para um pedido já armazenado.
	if id <= 0 || version <= 0 || createdAt.IsZero() || !status.valid() {
		return Order{}, ErrInvalidStoredOrder
	}

	// Reaproveitar NewOrder mantém em um único lugar as invariantes de cliente,
	// presença de itens, cálculo do total e proteção contra overflow.
	base, err := NewOrder(customer, items)
	if err != nil {
		return Order{}, fmt.Errorf("%w: %v", ErrInvalidStoredOrder, err)
	}
	// O total armazenado é conferido contra o valor recalculado. Uma divergência
	// indica dado corrompido ou gravado por uma versão incompatível do sistema.
	if base.totalCents != totalCents {
		return Order{}, ErrInvalidStoredOrder
	}

	base.id = id
	base.status = status
	base.version = version
	base.createdAt = createdAt
	return base, nil
}

// Pay realiza a transição PENDENTE → PAGO. A regra fica na entidade para valer
// igualmente quando o caso de uso for chamado por HTTP, CLI, teste ou outro meio.
func (order *Order) Pay() error {
	if order.status != StatusPending {
		return ErrInvalidStatusTransition
	}
	order.status = StatusPaid
	return nil
}

// Cancel realiza a transição PENDENTE → CANCELADO. Pedidos pagos ou já
// cancelados são estados finais e devolvem ErrInvalidStatusTransition.
func (order *Order) Cancel() error {
	if order.status != StatusPending {
		return ErrInvalidStatusTransition
	}
	order.status = StatusCanceled
	return nil
}

// ID devolve a identidade persistida do pedido; antes de Create ela é zero.
func (order Order) ID() int64 {
	return order.id
}

// Customer devolve o nome normalizado do cliente do pedido.
func (order Order) Customer() string {
	return order.customer
}

// Status devolve o estado atual, que só pode mudar por operações do domínio.
func (order Order) Status() Status {
	return order.status
}

// TotalCents devolve o total calculado pelo domínio a partir dos itens.
func (order Order) TotalCents() int64 {
	return order.totalCents
}

// Version devolve a versão usada pela persistência para detectar atualizações
// concorrentes e impedir que uma gravação antiga sobrescreva outra mais nova.
func (order Order) Version() int {
	return order.version
}

// CreatedAt devolve o instante definido pelo PostgreSQL ao inserir o pedido.
func (order Order) CreatedAt() time.Time {
	return order.createdAt
}

// Items devolve uma cópia do slice. Sem essa cópia, um chamador poderia trocar
// posições ou elementos do slice interno sem autorização da raiz do agregado.
func (order Order) Items() []Item {
	return copyItems(order.items)
}

// calculateTotal soma os subtotais e verifica overflow também entre itens. Um
// subtotal isolado pode caber em int64 e, ainda assim, a soma do pedido não caber.
func calculateTotal(items []Item) (int64, error) {
	var total int64
	for _, item := range items {
		subtotal := item.SubtotalCents()
		if total > math.MaxInt64-subtotal {
			return 0, ErrOrderTotalOverflow
		}
		total += subtotal
	}
	return total, nil
}

// copyItems centraliza a cópia defensiva usada na entrada e na saída do agregado.
func copyItems(items []Item) []Item {
	result := make([]Item, len(items))
	copy(result, items)
	return result
}

// valid confirma se um valor vindo de fora do processo, como uma linha do
// banco, corresponde a um dos estados reconhecidos pelo domínio.
func (status Status) valid() bool {
	switch status {
	case StatusPending, StatusPaid, StatusCanceled:
		return true
	default:
		return false
	}
}
