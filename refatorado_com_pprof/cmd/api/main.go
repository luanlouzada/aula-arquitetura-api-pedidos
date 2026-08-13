// Command api inicializa a API de pedidos e sua interface administrativa de
// profiling. A aplicação e o domínio permanecem iguais à versão refatorada;
// somente o composition root conhece o servidor pprof.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"aula-pedidos/refatorado_com_pprof/internal/application"
	"aula-pedidos/refatorado_com_pprof/internal/config"
	"aula-pedidos/refatorado_com_pprof/internal/httpapi"
	postgresadapter "aula-pedidos/refatorado_com_pprof/internal/infrastructure/postgres"
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

// run monta os mesmos componentes funcionais de refatorado e acrescenta um
// segundo servidor HTTP, isolado da API pública, para os handlers de pprof.
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

	// A composição funcional é a mesma de refatorado: a infraestrutura
	// implementa o contrato da aplicação e o service é entregue ao HTTP.
	orderRepository := postgresadapter.NewOrderRepository(database)
	orderService := application.NewOrderService(orderRepository)
	router := httpapi.NewRouter(orderService)

	apiServer := &http.Server{
		Addr:              settings.APIAddress,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	pprofServer := &http.Server{
		Addr:              settings.PprofAddress,
		Handler:           newPprofHandler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("API refatorada disponível em http://%s", settings.APIAddress)
	log.Printf("pprof disponível em http://%s/debug/pprof/", settings.PprofAddress)

	// Os dois servidores observam o mesmo processo. O primeiro erro inesperado
	// encerra a execução em vez de deixar apenas uma das interfaces disponível.
	serverErrors := make(chan error, 2)
	go func() {
		serverErrors <- serveHTTP(apiServer, "API pública")
	}()
	go func() {
		serverErrors <- serveHTTP(pprofServer, "servidor pprof")
	}()
	return <-serverErrors
}

// serveHTTP considera http.ErrServerClosed um encerramento normal e acrescenta
// contexto aos demais erros de listener.
func serveHTTP(server *http.Server, name string) error {
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("executar %s: %w", name, err)
	}
	return nil
}
