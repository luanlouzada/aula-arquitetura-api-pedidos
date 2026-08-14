// Command api monta a fila, o pool de cozinheiros e a interface HTTP
// usada para gerar carga. O preparo é simulado; o objetivo é tornar capacidade
// e sobrecarga observáveis.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"exercicio/internal/config"
	"exercicio/internal/httpapi"
	"exercicio/internal/kitchen"
)

func main() {
	if err := run(); err != nil {
		log.Printf("aplicação encerrada: %v", err)
		os.Exit(1)
	}
}

func run() error {
	settings, err := config.Load()
	if err != nil {
		return err
	}

	queue := kitchen.NewQueue(settings.QueueCapacity)
	processor := kitchen.Cook(settings.FixedCost, settings.PerTicketCost)
	pool, err := kitchen.NewPool(queue.Tickets(), settings.Cooks, processor)
	if err != nil {
		return fmt.Errorf("montar pool de cozinheiros: %w", err)
	}

	workerContext, cancelWorkers := context.WithCancel(context.Background())
	defer cancelWorkers()
	pool.Start(workerContext)

	handler := httpapi.NewHandler(queue)
	server := &http.Server{
		Addr:              settings.APIAddress,
		Handler:           handler.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("cozinha aberta em %s (cozinheiros=%d capacidade_fila=%d)",
		settings.APIAddress, settings.Cooks, settings.QueueCapacity)

	serverErrors := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	signalContext, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	var runErr error
	select {
	case <-signalContext.Done():
		log.Printf("encerramento solicitado")
	case runErr = <-serverErrors:
		log.Printf("servidor HTTP falhou: %v", runErr)
	}

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), settings.ShutdownTimeout)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownContext); err != nil {
		_ = server.Close()
		if runErr == nil {
			runErr = fmt.Errorf("encerrar HTTP: %w", err)
		}
	}
	queue.Close()

	drained := make(chan struct{})
	go func() {
		pool.Wait()
		close(drained)
	}()
	select {
	case <-drained:
		log.Printf("fila drenada")
	case <-shutdownContext.Done():
		cancelWorkers()
		<-drained
		if runErr == nil {
			runErr = errors.New("tempo de drenagem da fila excedido")
		}
	}
	return runErr
}
