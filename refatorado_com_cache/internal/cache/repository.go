package cache

import (
	"context"
	"log"
	"time"

	"aula-pedidos/refatorado_com_cache/internal/application"
	"aula-pedidos/refatorado_com_cache/internal/domain"
)

// OrderRepository decora o Repository PostgreSQL. A aplicação continua usando
// o mesmo contrato; o wrapper acrescenta cache somente às operações adequadas.
type OrderRepository struct {
	next    application.OrderRepository
	cache   OrderCache
	metrics *Metrics
	latency *LatencyWindow
}

func NewOrderRepository(
	next application.OrderRepository,
	orderCache OrderCache,
	metrics *Metrics,
	latency *LatencyWindow,
) *OrderRepository {
	return &OrderRepository{next: next, cache: orderCache, metrics: metrics, latency: latency}
}

// Create grava primeiro na fonte de verdade. O resultado ainda não é colocado
// no cache: no cache-aside, somente uma leitura solicitada aquece a chave.
func (repository *OrderRepository) Create(ctx context.Context, order domain.Order) (domain.Order, error) {
	return repository.next.Create(ctx, order)
}

// FindByID implementa cache-aside. Hit evita o PostgreSQL; miss carrega a fonte
// de verdade e tenta armazenar a cópia para a próxima leitura. Redis é tratado
// como otimização: uma falha é contada e a consulta continua pelo banco.
func (repository *OrderRepository) FindByID(ctx context.Context, id int64) (domain.Order, error) {
	startedAt := time.Now()
	order, found, err := repository.cache.Get(ctx, id)
	if err != nil {
		repository.metrics.RecordError()
		log.Printf("ler cache do pedido %d: %v", id, err)
	} else if found {
		repository.metrics.RecordHit()
		repository.latency.RecordHit(time.Since(startedAt))
		return order, nil
	} else {
		repository.metrics.RecordMiss()
	}

	order, err = repository.next.FindByID(ctx, id)
	if err != nil {
		repository.latency.RecordMiss(time.Since(startedAt))
		return domain.Order{}, err
	}
	if err := repository.cache.Set(ctx, order); err != nil {
		repository.metrics.RecordError()
		log.Printf("preencher cache do pedido %d: %v", id, err)
	}
	repository.latency.RecordMiss(time.Since(startedAt))
	return order, nil
}

// Save persiste primeiro no PostgreSQL e só então remove a cópia anterior. A
// próxima leitura será miss e recarregará o pedido com status e versão novos.
// Se a invalidação falhar, a escrita continua válida, mas pode haver staleness
// até o TTL; por isso a falha é registrada e observada.
func (repository *OrderRepository) Save(ctx context.Context, order domain.Order) (domain.Order, error) {
	persisted, err := repository.next.Save(ctx, order)
	if err != nil {
		return domain.Order{}, err
	}
	if err := repository.cache.Delete(ctx, persisted.ID()); err != nil {
		repository.metrics.RecordError()
		log.Printf("invalidar cache do pedido %d: %v", persisted.ID(), err)
	}
	return persisted, nil
}

var _ application.OrderRepository = (*OrderRepository)(nil)
