// Command api monta uma fila em memória, um pool de workers e a interface HTTP
// usada para gerar cenários de carga. O domínio simulado é gerar um artefato de
// pedido; o objetivo do código é tornar capacidade e sobrecarga observáveis.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"aula-fila-worker-pool/internal/config"
	"aula-fila-worker-pool/internal/httpapi"
	"aula-fila-worker-pool/internal/jobs"
	"aula-fila-worker-pool/internal/telemetry"
)

// main converte o erro final da aplicação em log e código de saída diferente de
// zero. A montagem permanece em run para que o fluxo principal possa retornar
// erros normalmente, sem chamar os.Exit em vários pontos.
func main() {
	if err := run(); err != nil {
		slog.Error("aplicação encerrada", slog.Any("error", err))
		os.Exit(1)
	}
}

// run é o composition root, ou ponto de montagem: lê a configuração, cria os
// componentes concretos, conecta HTTP, fila, workers e métricas, e controla o
// ciclo de vida do processo. Packages internos não criam suas próprias
// dependências globais.
func run() error {
	settings, err := config.Load()
	if err != nil {
		return err
	}
	// JSON estruturado permite filtrar campos como worker_id e batch_size sem
	// interpretar frases do log.
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Estes objetos são compartilhados pelas duas partes do fluxo: o Handler
	// produz Jobs e os workers os consomem; ambos atualizam as mesmas métricas.
	queue := jobs.NewQueue(settings.QueueCapacity)
	metrics := &telemetry.Metrics{}
	processor := jobs.NewSimulatedProcessor(settings.FixedCost, settings.PerJobCost)
	pool, err := jobs.NewPool(
		queue.Jobs(),
		settings.Workers,
		settings.BatchSize,
		settings.BatchWait,
		processor,
		metrics,
		logger,
	)
	if err != nil {
		return fmt.Errorf("montar worker pool: %w", err)
	}

	// O contexto permite interromper os workers se o prazo de shutdown terminar.
	// No encerramento normal, fechar a fila é suficiente para eles drenarem e sair.
	workerContext, cancelWorkers := context.WithCancel(context.Background())
	defer cancelWorkers()
	pool.Start(workerContext)

	// O Handler recebe dependências prontas. Ele não cria fila nem worker pool e
	// não conhece o Processor que executará os trabalhos.
	handler := httpapi.NewHandler(queue, metrics, settings.Workers, logger)
	server := &http.Server{
		Addr:              settings.APIAddress,
		Handler:           handler.Routes(),
		ReadHeaderTimeout: 3 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	logger.Info("laboratório iniciado",
		slog.String("address", settings.APIAddress),
		slog.Int("workers", settings.Workers),
		slog.Int("queue_capacity", settings.QueueCapacity),
		slog.Int("batch_size", settings.BatchSize),
		slog.Duration("batch_wait", settings.BatchWait),
		slog.Duration("fixed_cost", settings.FixedCost),
		slog.Duration("per_job_cost", settings.PerJobCost),
	)

	// ListenAndServe bloqueia, por isso roda em outra goroutine. O channel possui
	// buffer para que a goroutine consiga informar uma falha mesmo enquanto run
	// está tratando um sinal do sistema.
	serverErrors := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	// Ctrl+C envia os.Interrupt. SIGTERM é o sinal normalmente usado por Docker,
	// Kubernetes e serviços Linux para pedir um encerramento gracioso.
	signalContext, stopSignals := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stopSignals()

	var runErr error
	select {
	case <-signalContext.Done():
		logger.Info("encerramento solicitado")
	case runErr = <-serverErrors:
		logger.Error("servidor HTTP falhou", slog.Any("error", runErr))
	}

	// Primeiro a API deixa de produzir. Depois a fila é fechada e os workers
	// drenam o que já foi aceito. Essa ordem evita enviar para channel fechado.
	shutdownContext, cancelShutdown := context.WithTimeout(
		context.Background(),
		settings.ShutdownTimeout,
	)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownContext); err != nil {
		_ = server.Close()
		if runErr == nil {
			runErr = fmt.Errorf("encerrar HTTP: %w", err)
		}
	}
	queue.Close()

	// Fechar drained não transporta dados; apenas avisa que Wait terminou. Essa
	// espera concorre com o timeout para que o processo não fique preso para sempre.
	drained := make(chan struct{})
	go func() {
		pool.Wait()
		close(drained)
	}()
	select {
	case <-drained:
		logger.Info("fila drenada")
	case <-shutdownContext.Done():
		cancelWorkers()
		<-drained
		if runErr == nil {
			runErr = errors.New("tempo de drenagem da fila excedido")
		}
	}
	return runErr
}
