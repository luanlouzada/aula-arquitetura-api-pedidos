// Package redis implementa o cache de pedidos usando Redis e JSON.
package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	cacheport "aula-pedidos/refatorado_com_cache/internal/cache"
	"aula-pedidos/refatorado_com_cache/internal/domain"
	redisclient "github.com/redis/go-redis/v9"
)

const orderKeyFormatVersion = "v1"

// OrderCache conhece o cliente Redis, o TTL e a representação persistida. O
// domínio e o caso de uso não recebem nenhum desses detalhes.
type OrderCache struct {
	client *redisclient.Client
	ttl    time.Duration
}

func NewOrderCache(client *redisclient.Client, ttl time.Duration) *OrderCache {
	return &OrderCache{client: client, ttl: ttl}
}

// orderCacheEntry é um formato privado do cache. Alterar sua estrutura exige
// mudar a versão da chave para que bytes antigos não sejam lidos como o modelo
// novo. Isso é diferente do campo Version do agregado.
type orderCacheEntry struct {
	ID         int64                 `json:"id"`
	Customer   string                `json:"customer"`
	Status     domain.Status         `json:"status"`
	TotalCents int64                 `json:"total_cents"`
	Version    int                   `json:"version"`
	CreatedAt  time.Time             `json:"created_at"`
	Items      []orderItemCacheEntry `json:"items"`
}

type orderItemCacheEntry struct {
	ProductName    string `json:"product_name"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	Quantity       int    `json:"quantity"`
}

// Get transforma Redis Nil em miss. Em um hit, o JSON passa novamente pelos
// construtores do domínio; dados corrompidos não viram um Order válido.
func (cache *OrderCache) Get(ctx context.Context, id int64) (domain.Order, bool, error) {
	payload, err := cache.client.Get(ctx, orderKey(id)).Bytes()
	if err == redisclient.Nil {
		return domain.Order{}, false, nil
	}
	if err != nil {
		return domain.Order{}, false, fmt.Errorf("GET: %w", err)
	}

	var entry orderCacheEntry
	if err := json.Unmarshal(payload, &entry); err != nil {
		return domain.Order{}, false, fmt.Errorf("decodificar JSON: %w", err)
	}
	order, err := entry.toDomain()
	if err != nil {
		return domain.Order{}, false, fmt.Errorf("reconstituir pedido: %w", err)
	}
	return order, true, nil
}

// Set serializa uma cópia do agregado e usa o TTL configurado. Redis não passa
// a ser fonte de verdade: a entrada pode expirar ou ser removida a qualquer hora.
func (cache *OrderCache) Set(ctx context.Context, order domain.Order) error {
	payload, err := json.Marshal(entryFromDomain(order))
	if err != nil {
		return fmt.Errorf("codificar JSON: %w", err)
	}
	if err := cache.client.Set(ctx, orderKey(order.ID()), payload, cache.ttl).Err(); err != nil {
		return fmt.Errorf("SET: %w", err)
	}
	return nil
}

func (cache *OrderCache) Delete(ctx context.Context, id int64) error {
	if err := cache.client.Del(ctx, orderKey(id)).Err(); err != nil {
		return fmt.Errorf("DEL: %w", err)
	}
	return nil
}

// orderKey cria um namespace legível. v1 versiona a representação guardada;
// uma futura forma incompatível pode usar v2 e deixar v1 expirar pelo TTL.
func orderKey(id int64) string {
	return fmt.Sprintf("pedidos:orders:%s:%d", orderKeyFormatVersion, id)
}

func entryFromDomain(order domain.Order) orderCacheEntry {
	domainItems := order.Items()
	items := make([]orderItemCacheEntry, 0, len(domainItems))
	for _, item := range domainItems {
		items = append(items, orderItemCacheEntry{
			ProductName:    item.ProductName(),
			UnitPriceCents: item.UnitPriceCents(),
			Quantity:       item.Quantity(),
		})
	}
	return orderCacheEntry{
		ID:         order.ID(),
		Customer:   order.Customer(),
		Status:     order.Status(),
		TotalCents: order.TotalCents(),
		Version:    order.Version(),
		CreatedAt:  order.CreatedAt(),
		Items:      items,
	}
}

func (entry orderCacheEntry) toDomain() (domain.Order, error) {
	items := make([]domain.Item, 0, len(entry.Items))
	for _, cachedItem := range entry.Items {
		item, err := domain.NewItem(cachedItem.ProductName, cachedItem.UnitPriceCents, cachedItem.Quantity)
		if err != nil {
			return domain.Order{}, err
		}
		items = append(items, item)
	}
	return domain.RestoreOrder(
		entry.ID,
		entry.Customer,
		entry.Status,
		entry.TotalCents,
		entry.Version,
		entry.CreatedAt,
		items,
	)
}

var _ cacheport.OrderCache = (*OrderCache)(nil)
