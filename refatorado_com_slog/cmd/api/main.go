// Command api é o composition root da versão com logs estruturados. Ele cria
// os objetos concretos e conecta configuração, logger, banco, casos de uso e
// HTTP sem levar essas responsabilidades para o domínio.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"aula-pedidos/refatorado_com_slog/internal/application"
	"aula-pedidos/refatorado_com_slog/internal/config"
	"aula-pedidos/refatorado_com_slog/internal/httpapi"
	postgresadapter "aula-pedidos/refatorado_com_slog/internal/infrastructure/postgres"
	"aula-pedidos/refatorado_com_slog/internal/logging"
	"github.com/jackc/pgx/v5/pgxpool"
)

// main registra uma falha de inicialização ou execução e termina com um código
// de erro. slog.Error apenas escreve o evento; os.Exit preserva o comportamento
// de encerrar com status diferente de zero que antes era fornecido por log.Fatal.
func main() {
	if err := run(); err != nil {
		slog.Error("encerrar API", slog.Any("error", err))
		os.Exit(1)
	}
}

// run deixa explícita a ordem de montagem da aplicação. Cada objeto recebe
// somente as dependências necessárias, já prontas para uso.
func run() error {
	// O ambiente é lido uma única vez. Os demais pacotes recebem valores já
	// convertidos em vez de chamar os.Getenv por conta própria.
	settings, err := config.Load()
	if err != nil {
		return fmt.Errorf("carregar configuração: %w", err)
	}

	// O mesmo logger é compartilhado pela aplicação inteira. NewLogger fixa
	// campos comuns e configura o formato JSON e o nível mínimo.
	logger := logging.NewLogger(
		os.Stdout,
		settings.ServiceName,
		settings.Environment,
		settings.LogLevel,
	)
	slog.SetDefault(logger)

	// O timeout evita que uma dependência indisponível bloqueie o startup para
	// sempre. New prepara o pool; Ping confirma a conexão com o PostgreSQL.
	startupContext, cancelStartup := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelStartup()

	database, err := pgxpool.New(startupContext, settings.DatabaseURL)
	if err != nil {
		return fmt.Errorf("preparar pool PostgreSQL: %w", err)
	}
	defer database.Close()
	if err := database.Ping(startupContext); err != nil {
		return fmt.Errorf("conectar ao PostgreSQL: %w", err)
	}

	// Primeiro são criados os componentes funcionais. O decorator acrescenta
	// logs de negócio ao Service sem modificar as regras nem a persistência.
	orderRepository := postgresadapter.NewOrderRepository(database)
	orderService := application.NewOrderService(orderRepository)
	loggedOrderService := logging.NewLoggedOrderService(orderService, logger)

	// Controller recebe o Service e o logger por injeção. AccessLog envolve o
	// Router para registrar método, rota, status, bytes e duração.
	router := httpapi.NewRouter(loggedOrderService, logger)
	handler := httpapi.AccessLog(logger, router)

	server := &http.Server{
		Addr:              settings.APIAddress,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	logger.Info(
		"API disponível",
		slog.String("address", "http://"+settings.APIAddress),
		slog.String("log_level", settings.LogLevel.String()),
	)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("executar servidor HTTP: %w", err)
	}
	return nil
}
