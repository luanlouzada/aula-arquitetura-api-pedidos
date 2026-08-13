package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	legacyapi "aula-pedidos/legado/internal/api"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultDatabaseURL = "postgres://postgres:postgres@localhost:5435/pedidos?sslmode=disable"
	defaultAPIAddress  = "127.0.0.1:8080"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// LIMITE SAUDÁVEL: main é o ponto de montagem da aplicação, também chamado
	// de composition root. Seu trabalho é criar os objetos concretos e conectá-los.
	// Por isso é normal que ele conheça configuração, PostgreSQL e servidor HTTP.
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = defaultDatabaseURL
	}

	address := os.Getenv("API_ADDR")
	if address == "" {
		address = defaultAPIAddress
	}

	connectionContext, cancelConnection := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelConnection()

	// Criar o pool PostgreSQL aqui é adequado: estamos inicializando a aplicação.
	// Seria problemático se o cálculo do total ou a regra de pagamento precisassem
	// importar pgx para funcionar.
	database, err := pgxpool.New(connectionContext, databaseURL)
	if err != nil {
		return fmt.Errorf("preparar pool PostgreSQL: %w", err)
	}
	defer database.Close()

	if err := database.Ping(connectionContext); err != nil {
		return fmt.Errorf("conectar ao PostgreSQL: %w", err)
	}

	// Aqui conectar banco e servidor é correto. O problema do legado aparece no
	// próximo passo: legacyapi.New recebe o pool e os próprios handlers executam SQL.
	// Assim, o detalhe do PostgreSQL não fica somente neste ponto de inicialização.
	server := &http.Server{
		Addr:              address,
		Handler:           legacyapi.New(database),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("API legado disponível em http://%s", address)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("executar servidor HTTP: %w", err)
	}
	return nil
}
