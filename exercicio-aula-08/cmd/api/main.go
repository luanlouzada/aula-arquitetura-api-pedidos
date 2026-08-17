// Command api monta uma instância da central de exportações. Os componentes
// estão conectados; os TODOs estão nas políticas que o exercício pede.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"exercicio-aula-08/internal/admission"
	"exercicio-aula-08/internal/config"
	"exercicio-aula-08/internal/exports"
	"exercicio-aula-08/internal/health"
	"exercicio-aula-08/internal/httpapi"
	"exercicio-aula-08/internal/lifecycle"
	"exercicio-aula-08/internal/telemetry"
)

// main converte o erro final em código de saída. A execução fica em run para que
// defers de cancelamento ainda sejam executados antes de os.Exit.
func main() {
	if err := run(); err != nil {
		slog.Error("aplicação encerrada", slog.Any("error", err))
		os.Exit(1)
	}
}

// run é o composition root: lê configuração, cria objetos concretos, conecta o
// fluxo HTTP -> admissão -> fila -> workers e coordena o ciclo de vida.
func run() error {
	// A captura vem antes da montagem. Desse modo, SIGTERM recebido durante a
	// inicialização fica pendente no contexto e segue pelo encerramento coordenado,
	// em vez de aplicar a saída abrupta padrão do sistema operacional.
	signalContext, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	settings, err := config.Load()
	if err != nil {
		return err
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	state := health.NewState()
	limiter, err := admission.NewTokenBucket(settings.RateLimitRPS, settings.RateLimitBurst)
	if err != nil {
		return fmt.Errorf("criar token bucket: %w", err)
	}
	queue := exports.NewQueue(settings.QueueCapacity)
	metrics := telemetry.New(settings.InstanceID)
	generator := exports.NewSimulatedGenerator(settings.GenerationTime)
	pool, err := exports.NewPool(queue.Exports(), settings.Workers, generator, metrics)
	if err != nil {
		return fmt.Errorf("criar pool de exportação: %w", err)
	}

	workerContext, cancelWorkers := context.WithCancel(context.Background())
	defer cancelWorkers()
	pool.Start(workerContext)

	handler := httpapi.NewHandler(
		settings.InstanceID,
		state,
		limiter,
		queue,
		metrics,
		settings.Workers,
		settings.RateLimitRPS,
		settings.RateLimitBurst,
	)
	server := &http.Server{
		Addr:              settings.APIAddress,
		Handler:           handler.Routes(),
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	// net.Listen abre a porta antes de MarkReady. Se o endereço estiver ocupado,
	// a aplicação falha sem anunciar prontidão por engano.
	listener, err := net.Listen("tcp", settings.APIAddress)
	if err != nil {
		return fmt.Errorf("abrir listener HTTP em %s: %w", settings.APIAddress, err)
	}
	serverErrors := make(chan error, 1)
	// Serve bloqueia enquanto atende. A goroutine permite que run espere ao mesmo
	// tempo uma falha do servidor ou um sinal do sistema operacional.
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()
	// A prontidão só é anunciada depois que fila, workers, rotas e servidor foram
	// preparados e se o contexto ainda não registrou um sinal. O TODO de State deve
	// fazer esta transição ficar observável.
	if signalContext.Err() == nil {
		state.MarkReady()
		logger.Info("exportador pronto",
			slog.String("instance_id", settings.InstanceID),
			slog.String("address", settings.APIAddress),
		)
	}

	var runErr error
	select {
	case <-signalContext.Done():
		logger.Info("SIGTERM ou interrupção recebida", slog.String("instance_id", settings.InstanceID))
	case runErr = <-serverErrors:
		logger.Error("servidor HTTP falhou", slog.Any("error", runErr))
	}

	// O contexto de shutdown nasce de Background deliberadamente: o contexto de
	// sinal já está cancelado e encerraria a drenagem no mesmo instante.
	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), settings.ShutdownTimeout)
	defer cancelShutdown()
	shutdownErr := lifecycle.Shutdown(shutdownContext, settings.DrainDelay, lifecycle.Steps{
		MarkNotReady:     state.MarkNotReady,
		ShutdownHTTP:     server.Shutdown,
		ForceCloseHTTP:   server.Close,
		CloseQueue:       queue.Close,
		WaitWorkers:      pool.Wait,
		ForceStopWorkers: cancelWorkers,
	})
	return errors.Join(runErr, shutdownErr)
}
