package cache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"aula-pedidos/refatorado_com_cache/internal/cache"
	"aula-pedidos/refatorado_com_cache/internal/domain"
)

func TestFindByIDUsesDatabaseOnMissAndCacheOnNextRead(t *testing.T) {
	ctx := context.Background()
	order := restoredOrder(t, 1, domain.StatusPending, 1)
	database := &repositoryStub{order: order}
	orderCache := newCacheStub()
	metrics := &cache.Metrics{}
	repository := cache.NewOrderRepository(database, orderCache, metrics, &cache.LatencyWindow{})

	first, err := repository.FindByID(ctx, order.ID())
	if err != nil {
		t.Fatalf("primeiro FindByID() error = %v", err)
	}
	second, err := repository.FindByID(ctx, order.ID())
	if err != nil {
		t.Fatalf("segundo FindByID() error = %v", err)
	}
	if first.ID() != order.ID() || second.ID() != order.ID() {
		t.Fatalf("FindByID() IDs = %d e %d, want %d", first.ID(), second.ID(), order.ID())
	}
	if database.findCalls != 1 {
		t.Fatalf("banco consultado %d vezes, want 1", database.findCalls)
	}
	snapshot := metrics.Snapshot()
	if snapshot.Misses != 1 || snapshot.Hits != 1 || snapshot.HitRatio != 0.5 {
		t.Fatalf("Snapshot() = %+v, want 1 miss, 1 hit e ratio 0.5", snapshot)
	}
}

func TestSavePersistsBeforeInvalidatingCache(t *testing.T) {
	ctx := context.Background()
	order := restoredOrder(t, 8, domain.StatusPending, 1)
	database := &repositoryStub{order: order}
	orderCache := newCacheStub()
	orderCache.orders[order.ID()] = order
	repository := cache.NewOrderRepository(database, orderCache, &cache.Metrics{}, &cache.LatencyWindow{})

	if _, err := repository.Save(ctx, order); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if database.saveCalls != 1 {
		t.Fatalf("banco salvo %d vezes, want 1", database.saveCalls)
	}
	if _, found := orderCache.orders[order.ID()]; found {
		t.Fatal("cache ainda contém a chave depois de Save()")
	}
}

func TestCacheFailureFallsBackToDatabase(t *testing.T) {
	ctx := context.Background()
	order := restoredOrder(t, 3, domain.StatusPending, 1)
	database := &repositoryStub{order: order}
	orderCache := newCacheStub()
	orderCache.getError = errors.New("Redis indisponível")
	metrics := &cache.Metrics{}
	repository := cache.NewOrderRepository(database, orderCache, metrics, &cache.LatencyWindow{})

	got, err := repository.FindByID(ctx, order.ID())
	if err != nil {
		t.Fatalf("FindByID() error = %v", err)
	}
	if got.ID() != order.ID() || database.findCalls != 1 {
		t.Fatalf("fallback = pedido %d e %d leituras, want pedido %d e 1 leitura", got.ID(), database.findCalls, order.ID())
	}
	if metrics.Snapshot().Errors != 1 {
		t.Fatalf("Errors = %d, want 1", metrics.Snapshot().Errors)
	}
}

type repositoryStub struct {
	order     domain.Order
	findCalls int
	saveCalls int
}

func (repository *repositoryStub) Create(_ context.Context, order domain.Order) (domain.Order, error) {
	return order, nil
}

func (repository *repositoryStub) FindByID(_ context.Context, _ int64) (domain.Order, error) {
	repository.findCalls++
	return repository.order, nil
}

func (repository *repositoryStub) Save(_ context.Context, order domain.Order) (domain.Order, error) {
	repository.saveCalls++
	return order, nil
}

type cacheStub struct {
	orders   map[int64]domain.Order
	getError error
}

func newCacheStub() *cacheStub { return &cacheStub{orders: make(map[int64]domain.Order)} }

func (stub *cacheStub) Get(_ context.Context, id int64) (domain.Order, bool, error) {
	if stub.getError != nil {
		return domain.Order{}, false, stub.getError
	}
	order, found := stub.orders[id]
	return order, found, nil
}

func (stub *cacheStub) Set(_ context.Context, order domain.Order) error {
	stub.orders[order.ID()] = order
	return nil
}

func (stub *cacheStub) Delete(_ context.Context, id int64) error {
	delete(stub.orders, id)
	return nil
}

func restoredOrder(t *testing.T, id int64, status domain.Status, version int) domain.Order {
	t.Helper()
	item, err := domain.NewItem("Livro", 5_000, 2)
	if err != nil {
		t.Fatal(err)
	}
	order, err := domain.RestoreOrder(
		id,
		"Ana",
		status,
		10_000,
		version,
		time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC),
		[]domain.Item{item},
	)
	if err != nil {
		t.Fatal(err)
	}
	return order
}
