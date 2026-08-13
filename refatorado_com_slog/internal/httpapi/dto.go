package httpapi

import "time"

// Este arquivo define DTOs (Data Transfer Objects): formatos feitos para o
// contrato HTTP. As tags json e os nomes em português pertencem à API e não
// precisam invadir as entidades nem os casos de uso.

// createOrderRequest representa exatamente o JSON aceito por POST /orders.
// Ele descreve formato, não validade de negócio; o domínio ainda verificará os
// valores depois que o mapper os encaminhar à aplicação.
type createOrderRequest struct {
	Customer string                   `json:"cliente"`
	Items    []createOrderItemRequest `json:"itens"`
}

// createOrderItemRequest representa cada objeto do array "itens" da requisição.
type createOrderItemRequest struct {
	ProductName    string `json:"produto"`
	UnitPriceCents int64  `json:"preco_unitario_centavos"`
	Quantity       int    `json:"quantidade"`
}

// orderResponse é o formato público devolvido pela API. Ele funciona como o
// modelo de apresentação do pedido: contém somente valores serializáveis e os
// nomes de campos prometidos ao cliente HTTP.
type orderResponse struct {
	ID         int64               `json:"id"`
	Customer   string              `json:"cliente"`
	Status     string              `json:"status"`
	TotalCents int64               `json:"total_centavos"`
	Version    int                 `json:"versao"`
	CreatedAt  time.Time           `json:"criado_em"`
	Items      []orderItemResponse `json:"itens"`
}

// orderItemResponse é a representação HTTP de um Item e inclui o subtotal já
// calculado pelo domínio.
type orderItemResponse struct {
	ProductName    string `json:"produto"`
	UnitPriceCents int64  `json:"preco_unitario_centavos"`
	Quantity       int    `json:"quantidade"`
	SubtotalCents  int64  `json:"subtotal_centavos"`
}

// errorResponse padroniza o corpo JSON usado nas respostas de erro.
type errorResponse struct {
	Error string `json:"erro"`
}
