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

	"aula-pedidos/refatorado/internal/application"
	"aula-pedidos/refatorado/internal/config"
	"aula-pedidos/refatorado/internal/httpapi"
	postgresadapter "aula-pedidos/refatorado/internal/infrastructure/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
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
	settings := config.Load()

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
	orderRepository := postgresadapter.NewOrderRepository(database)
	orderService := application.NewOrderService(orderRepository)
	router := httpapi.NewRouter(orderService)

	server := &http.Server{
		Addr:              settings.APIAddress,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("API refatorada disponível em http://%s", settings.APIAddress)
	// http.ErrServerClosed é o retorno normal quando um servidor é encerrado; os
	// demais erros representam falha real ao abrir ou manter o listener.
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("executar servidor HTTP: %w", err)
	}
	return nil
}
