// Package cache declara a fronteira mínima usada para armazenar cópias
// temporárias de pedidos. O contrato não menciona Redis, comandos ou JSON.
package cache

import (
	"context"

	"aula-pedidos/refatorado_com_cache/internal/domain"
)

// OrderCache representa três operações necessárias ao cache-aside: procurar,
// armazenar com a política de expiração da implementação e invalidar uma chave.
// O bool de Get distingue cache miss de um pedido encontrado.
type OrderCache interface {
	Get(ctx context.Context, id int64) (order domain.Order, found bool, err error)
	Set(ctx context.Context, order domain.Order) error
	Delete(ctx context.Context, id int64) error
}
