// Command api monta uma instância da API protegida por token bucket, fila
// limitada e worker pool, e coordena seu encerramento ao receber SIGTERM.
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

	"aula-08-confiabilidade/internal/admission"
	"aula-08-confiabilidade/internal/config"
	"aula-08-confiabilidade/internal/health"
	"aula-08-confiabilidade/internal/httpapi"
	"aula-08-confiabilidade/internal/jobs"
	"aula-08-confiabilidade/internal/lifecycle"
	"aula-08-confiabilidade/internal/telemetry"
)

// main é pequeno porque os.Exit interrompe defers. Toda a execução fica em run,
// que pode devolver erro normalmente; somente aqui escolhemos o código de saída.
func main() {
	if err := run(); err != nil {
		slog.Error("aplicação encerrada", slog.Any("error", err))
		os.Exit(1)
	}
}

// run é o composition root: cria implementações concretas, conecta suas
// dependências e controla quando cada parte começa e termina. Nenhum package
// interno precisa procurar dependências globais ou ler o ambiente novamente.
func run() error {
	// A captura de sinais precisa existir antes de criar workers, listener ou
	// prontidão. Assim, um SIGTERM recebido durante a inicialização fica registrado
	// no contexto e será tratado pelo mesmo fluxo de shutdown, em vez de encerrar o
	// processo pela ação padrão do sistema operacional.
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
	queue := jobs.NewQueue(settings.QueueCapacity)
	metrics := telemetry.New(settings.InstanceID)
	processor := jobs.NewSimulatedProcessor(settings.ProcessingTime)
	pool, err := jobs.NewPool(queue.Jobs(), settings.Workers, processor, metrics)
	if err != nil {
		return fmt.Errorf("criar worker pool: %w", err)
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

	// Abrir o listener de forma síncrona prova que o endereço está disponível.
	// Marcar ready antes desta chamada poderia anunciar uma instância cuja porta
	// falhou ao abrir. Serve usa o listener já confirmado.
	listener, err := net.Listen("tcp", settings.APIAddress)
	if err != nil {
		return fmt.Errorf("abrir listener HTTP em %s: %w", settings.APIAddress, err)
	}
	serverErrors := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()
	// Se o contexto já registrou um sinal durante a montagem, a instância não
	// anuncia prontidão. O select abaixo observa o cancelamento e inicia o drain.
	if signalContext.Err() == nil {
		state.MarkReady()
		logger.Info("instância pronta",
			slog.String("instance_id", settings.InstanceID),
			slog.String("address", settings.APIAddress),
			slog.Float64("rate_limit_rps", settings.RateLimitRPS),
			slog.Int("rate_limit_burst", settings.RateLimitBurst),
			slog.Int("queue_capacity", settings.QueueCapacity),
			slog.Int("workers", settings.Workers),
		)
	}

	var runErr error
	select {
	case <-signalContext.Done():
		logger.Info("SIGTERM ou interrupção recebida", slog.String("instance_id", settings.InstanceID))
	case runErr = <-serverErrors:
		logger.Error("servidor HTTP falhou", slog.Any("error", runErr))
	}

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), settings.ShutdownTimeout)
	defer cancelShutdown()
	// Os valores abaixo são method values: montar Steps apenas guarda referências
	// para métodos dos objetos criados acima. Nenhuma etapa é executada aqui. A
	// função lifecycle.Shutdown decide quando chamar cada uma e em qual ordem.
	shutdownErr := lifecycle.Shutdown(shutdownContext, settings.DrainDelay, lifecycle.Steps{
		MarkNotReady:     state.MarkNotReady,
		ShutdownHTTP:     server.Shutdown,
		ForceCloseHTTP:   server.Close,
		CloseQueue:       queue.Close,
		WaitWorkers:      pool.Wait,
		ForceStopWorkers: cancelWorkers,
	})
	if shutdownErr != nil {
		runErr = errors.Join(runErr, shutdownErr)
	} else {
		logger.Info("shutdown concluído: HTTP fechado e fila drenada", slog.String("instance_id", settings.InstanceID))
	}
	return runErr
}
