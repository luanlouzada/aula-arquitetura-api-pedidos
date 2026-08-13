// Command api inicializa e conecta todos os componentes concretos da API.
// Este pacote é o composition root: é esperado que ele conheça configuração,
// PostgreSQL, aplicação e HTTP, enquanto essas camadas permanecem desacopladas
// umas das outras nas direções que não são necessárias.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"aula-pedidos/refatorado_com_cache/internal/application"
	cachelayer "aula-pedidos/refatorado_com_cache/internal/cache"
	"aula-pedidos/refatorado_com_cache/internal/config"
	"aula-pedidos/refatorado_com_cache/internal/httpapi"
	postgresadapter "aula-pedidos/refatorado_com_cache/internal/infrastructure/postgres"
	redisadapter "aula-pedidos/refatorado_com_cache/internal/infrastructure/redis"
	"github.com/jackc/pgx/v5/pgxpool"
	redisclient "github.com/redis/go-redis/v9"
)

// main é a fronteira do processo. Ele transforma uma falha de inicialização ou
// execução em encerramento da aplicação; a função run permanece testável e pode
// devolver erros normalmente.
func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run controla o ciclo de inicialização: carrega configuração, valida o acesso
// ao banco, compõe dependências, configura o servidor e começa a escutar HTTP.
func run() error {
	settings, err := config.Load()
	if err != nil {
		return fmt.Errorf("carregar configuração: %w", err)
	}

	// O timeout impede que a inicialização espere indefinidamente por rede ou
	// PostgreSQL. O mesmo contexto é usado para criar e verificar o pool.
	connectionContext, cancelConnection := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelConnection()

	database, err := pgxpool.New(connectionContext, settings.DatabaseURL)
	if err != nil {
		return fmt.Errorf("preparar pool PostgreSQL: %w", err)
	}
	defer database.Close()

	if err := database.Ping(connectionContext); err != nil {
		return fmt.Errorf("conectar ao PostgreSQL: %w", err)
	}

	// Composition root: a infraestrutura implementa o contrato da aplicação; o
	// service é entregue ao adaptador HTTP. Nenhuma dessas camadas precisa criar
	// ou localizar sua própria dependência.
	postgresRepository := postgresadapter.NewOrderRepository(database)
	cacheMetrics := &cachelayer.Metrics{}
	cacheLatency := &cachelayer.LatencyWindow{}
	var orderRepository application.OrderRepository = postgresRepository

	if settings.CacheEnabled {
		// Redis é uma dependência de desempenho, não a fonte de verdade. O Ping
		// torna o laboratório previsível antes da medição de hits e misses.
		redisDatabase := redisclient.NewClient(&redisclient.Options{
			Addr:                  settings.RedisAddress,
			DialTimeout:           time.Second,
			ReadTimeout:           500 * time.Millisecond,
			WriteTimeout:          500 * time.Millisecond,
			ContextTimeoutEnabled: true,
		})
		defer redisDatabase.Close()
		if err := redisDatabase.Ping(connectionContext).Err(); err != nil {
			return fmt.Errorf("conectar ao Redis: %w", err)
		}

		redisOrderCache := redisadapter.NewOrderCache(redisDatabase, settings.CacheTTL)
		orderRepository = cachelayer.NewOrderRepository(
			postgresRepository,
			redisOrderCache,
			cacheMetrics,
			cacheLatency,
		)
	}

	orderService := application.NewOrderService(orderRepository)
	router := httpapi.NewRouter(orderService, cacheMetrics.Snapshot, cacheLatency.Snapshot)

	server := &http.Server{
		Addr:              settings.APIAddress,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("API disponível em http://%s", settings.APIAddress)
	if settings.CacheEnabled {
		log.Printf("cache Redis em %s com TTL %s", settings.RedisAddress, settings.CacheTTL)
	} else {
		log.Printf("cache desativado: linha de base usando PostgreSQL")
	}
	// http.ErrServerClosed é o retorno normal quando um servidor é encerrado; os
	// demais erros representam falha real ao abrir ou manter o listener.
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("executar servidor HTTP: %w", err)
	}
	return nil
}
