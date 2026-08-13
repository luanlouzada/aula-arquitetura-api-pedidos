package httpapi

import (
	"aula-pedidos/refatorado_com_pprof/internal/application"
	"aula-pedidos/refatorado_com_pprof/internal/domain"
)

// Este arquivo concentra conversões entre representações. Mappers copiam e
// reorganizam dados na fronteira; eles não validam transições nem calculam
// regras que pertencem ao domínio.

// toCreateOrderInput remove da requisição a representação específica de JSON e
// produz a entrada esperada pelo caso de uso. Ele não cria domain.Order: essa
// responsabilidade permanece com os construtores chamados pela aplicação.
func toCreateOrderInput(request createOrderRequest) application.CreateOrderInput {
	items := make([]application.CreateOrderItemInput, 0, len(request.Items))
	for _, item := range request.Items {
		items = append(items, application.CreateOrderItemInput{
			ProductName:    item.ProductName,
			UnitPriceCents: item.UnitPriceCents,
			Quantity:       item.Quantity,
		})
	}
	return application.CreateOrderInput{
		Customer: request.Customer,
		Items:    items,
	}
}

// toOrderResponse transforma o agregado em um DTO próprio para HTTP. Este é o
// papel de apresentação da API: expor strings, datas e campos JSON sem obrigar
// Order a conhecer como será entregue ao cliente.
func toOrderResponse(order domain.Order) orderResponse {
	// Esta versão contém o gargalo investigado no laboratório. Items devolve
	// uma cópia defensiva do slice interno. Guardamos uma cópia para conhecer o
	// tamanho, mas voltamos a chamar o getter em cada iteração. O JSON continua
	// correto, porém um pedido grande provoca cópias e alocações repetidas.
	orderItems := order.Items()
	items := make([]orderItemResponse, 0, len(orderItems))
	for index := range orderItems {
		item := order.Items()[index]
		items = append(items, orderItemResponse{
			ProductName:    item.ProductName(),
			UnitPriceCents: item.UnitPriceCents(),
			Quantity:       item.Quantity(),
			SubtotalCents:  item.SubtotalCents(),
		})
	}
	return orderResponse{
		ID:         order.ID(),
		Customer:   order.Customer(),
		Status:     string(order.Status()),
		TotalCents: order.TotalCents(),
		Version:    order.Version(),
		CreatedAt:  order.CreatedAt(),
		Items:      items,
	}
}
