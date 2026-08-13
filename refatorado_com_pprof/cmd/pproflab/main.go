// Command pproflab produz gargalos controlados para estudar os perfis do Go.
// Ele é um programa isolado: não usa a API de pedidos, PostgreSQL, domínio,
// Service ou Repository. Cada cenário executa uma função com uma característica
// diferente e expõe o próprio pprof em uma porta administrativa local.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	// pprof: o laboratório usa a mesma ponte HTTP para o runtime usada pela API.
	httppprof "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	// pprof: 6061 evita disputar a porta 6060 da API de pedidos.
	defaultPprofAddress = "127.0.0.1:6061"
	defaultDuration     = 60 * time.Second
	defaultMemoryMB     = 64
	defaultGoroutines   = 1000
)

// settings reúne somente os parâmetros usados pelas cargas artificiais.
// Os valores são flags para permitir uma demonstração curta ou mais intensa
// sem editar o código.
type settings struct {
	scenario     string
	pprofAddress string
	duration     time.Duration
	memoryMB     int
	goroutines   int
}

type workload func(context.Context, settings) error

var (
	// Os sinks tornam os resultados observáveis para o compilador. Sem isso, uma
	// computação cujo resultado nunca é usado poderia desaparecer da execução.
	cpuSink        atomic.Uint64
	ioSink         atomic.Uint64
	blockSink      atomic.Int64
	allocationSink []byte
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "pproflab:", err)
		os.Exit(1)
	}
}

func run() error {
	configured := settings{}
	flag.StringVar(
		&configured.scenario,
		"scenario",
		"cpu",
		"cenário: cpu, io, goroutine, memory, allocs, mutex ou block",
	)
	flag.StringVar(
		&configured.pprofAddress,
		"pprof-addr",
		defaultPprofAddress,
		"endereço local da interface pprof",
	)
	flag.DurationVar(
		&configured.duration,
		"duration",
		defaultDuration,
		"duração do cenário; zero executa até Ctrl+C",
	)
	flag.IntVar(
		&configured.memoryMB,
		"memory-mb",
		defaultMemoryMB,
		"megabytes retidos pelo cenário memory",
	)
	flag.IntVar(
		&configured.goroutines,
		"goroutines",
		defaultGoroutines,
		"quantidade criada pelo cenário goroutine",
	)
	flag.Parse()

	workloads := map[string]workload{
		"cpu":       cpuWork,
		"io":        ioWork,
		"goroutine": goroutineWork,
		"memory":    memoryWork,
		"allocs":    allocsWork,
		"mutex":     mutexWork,
		"block":     blockWork,
	}
	selected, exists := workloads[configured.scenario]
	if !exists {
		return fmt.Errorf(
			"scenario %q é inválido: use cpu, io, goroutine, memory, allocs, mutex ou block",
			configured.scenario,
		)
	}
	if configured.pprofAddress == "" {
		return errors.New("pprof-addr não pode ser vazio")
	}
	if configured.duration < 0 {
		return errors.New("duration não pode ser negativo")
	}
	if configured.memoryMB <= 0 || configured.goroutines <= 0 {
		return errors.New("memory-mb e goroutines devem ser maiores que zero")
	}

	// O contexto termina por tempo ou por Ctrl+C. As funções de carga recebem o
	// mesmo contexto para que liberem arquivos, goroutines e memória ao encerrar.
	signalContext, stopSignals := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stopSignals()

	workContext := signalContext
	cancelDuration := func() {}
	if configured.duration > 0 {
		workContext, cancelDuration = context.WithTimeout(
			signalContext,
			configured.duration,
		)
	}
	defer cancelDuration()
	workContext, cancelWork := context.WithCancel(workContext)
	defer cancelWork()

	// pprof: o programa isolado também expõe os handlers em um servidor local,
	// mas não cria nenhuma rota de negócio.
	server := &http.Server{
		Addr:              configured.pprofAddress,
		Handler:           newPprofHandler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serverResult := make(chan error, 1)
	go func() {
		err := server.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		serverResult <- err
	}()

	printInstructions(configured)

	workResult := make(chan error, 1)
	go func() {
		workResult <- selected(workContext, configured)
	}()

	var result error
	serverFinished := false
	select {
	case err := <-serverResult:
		serverFinished = true
		if err != nil {
			result = fmt.Errorf("executar servidor pprof: %w", err)
		} else {
			result = errors.New("servidor pprof encerrou antes da carga")
		}
	case err := <-workResult:
		if err != nil {
			result = fmt.Errorf("executar cenário %s: %w", configured.scenario, err)
		}
	}
	cancelWork()

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownContext); err != nil && result == nil {
		result = fmt.Errorf("encerrar servidor pprof: %w", err)
	}
	if !serverFinished {
		if err := <-serverResult; err != nil && result == nil {
			result = fmt.Errorf("encerrar servidor pprof: %w", err)
		}
	}

	if result == nil {
		fmt.Printf("\ncenário %s encerrado\n", configured.scenario)
	}
	return result
}

// newPprofHandler registra explicitamente os handlers no mux administrativo.
// Não usamos o DefaultServeMux global, mesmo neste laboratório, para deixar
// visível quais rotas existem e qual função da biblioteca padrão as atende.
func newPprofHandler() http.Handler {
	mux := http.NewServeMux()
	// pprof: estas são as mesmas funções da biblioteca padrão usadas pela API.
	mux.HandleFunc("GET /debug/pprof/", httppprof.Index)
	mux.HandleFunc("GET /debug/pprof/cmdline", httppprof.Cmdline)
	mux.HandleFunc("GET /debug/pprof/profile", httppprof.Profile)
	mux.HandleFunc("GET /debug/pprof/symbol", httppprof.Symbol)
	mux.HandleFunc("GET /debug/pprof/trace", httppprof.Trace)
	return mux
}

func printInstructions(configured settings) {
	baseURL := "http://" + configured.pprofAddress
	durationText := configured.duration.String()
	if configured.duration == 0 {
		durationText = "até Ctrl+C"
	}

	fmt.Printf(
		"cenário: %s\nduração: %s\npprof: %s/debug/pprof/\n\n",
		configured.scenario,
		durationText,
		baseURL,
	)
	fmt.Println("em outro terminal:")
	fmt.Println("  mkdir -p profiles")

	switch configured.scenario {
	case "cpu":
		fmt.Printf("  curl -fsS -o profiles/lab-cpu.pprof '%s/debug/pprof/profile?seconds=10'\n", baseURL)
		fmt.Println("  go tool pprof -top -cum profiles/lab-cpu.pprof")
	case "io":
		fmt.Printf("  curl -fsS -o profiles/lab-io.trace '%s/debug/pprof/trace?seconds=10'\n", baseURL)
		fmt.Println("  go tool trace profiles/lab-io.trace")
		fmt.Println("  CPU profile mostra pouco quando o processo passa mais tempo esperando I/O.")
	case "goroutine":
		fmt.Printf("  curl -fsS '%s/debug/pprof/goroutine?debug=1' | less\n", baseURL)
	case "memory":
		fmt.Printf("  curl -fsS -o profiles/lab-memory.pprof '%s/debug/pprof/heap?gc=1'\n", baseURL)
		fmt.Println("  go tool pprof -top -sample_index=inuse_space profiles/lab-memory.pprof")
	case "allocs":
		fmt.Printf("  curl -fsS -o profiles/lab-allocs.pprof '%s/debug/pprof/allocs?seconds=10'\n", baseURL)
		fmt.Println("  go tool pprof -top -sample_index=alloc_space profiles/lab-allocs.pprof")
	case "mutex":
		fmt.Printf("  curl -fsS -o profiles/lab-mutex.pprof '%s/debug/pprof/mutex?seconds=10'\n", baseURL)
		fmt.Println("  go tool pprof -top profiles/lab-mutex.pprof")
	case "block":
		fmt.Printf("  curl -fsS -o profiles/lab-block.pprof '%s/debug/pprof/block?seconds=10'\n", baseURL)
		fmt.Println("  go tool pprof -top profiles/lab-block.pprof")
	}
}

// cpuWork mantém um núcleo ocupado procurando números primos. O resultado é
// guardado em cpuSink para impedir que o compilador remova o cálculo.
func cpuWork(ctx context.Context, _ settings) error {
	var total uint64
	for ctx.Err() == nil {
		total += countPrimes(50_000)
		cpuSink.Store(total)
	}
	return nil
}

func countPrimes(limit int) uint64 {
	var count uint64
	for candidate := 2; candidate <= limit; candidate++ {
		prime := true
		for divisor := 2; divisor*divisor <= candidate; divisor++ {
			if candidate%divisor == 0 {
				prime = false
				break
			}
		}
		if prime {
			count++
		}
	}
	return count
}

// ioWork grava, sincroniza e lê um arquivo temporário. O arquivo é removido no
// encerramento. Esse cenário é mais útil no execution trace do que no CPU
// profile, pois o tempo esperando o sistema operacional não é CPU ativa.
func ioWork(ctx context.Context, _ settings) error {
	file, err := os.CreateTemp("", "pproflab-io-*")
	if err != nil {
		return fmt.Errorf("criar arquivo temporário: %w", err)
	}
	fileName := file.Name()
	defer os.Remove(fileName)
	defer file.Close()

	buffer := make([]byte, 64*1024)
	for index := range buffer {
		buffer[index] = byte(index)
	}
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := file.Truncate(0); err != nil {
				return fmt.Errorf("truncar arquivo temporário: %w", err)
			}
			if _, err := file.Seek(0, io.SeekStart); err != nil {
				return fmt.Errorf("reposicionar para escrita: %w", err)
			}
			if _, err := file.Write(buffer); err != nil {
				return fmt.Errorf("escrever arquivo temporário: %w", err)
			}
			if err := file.Sync(); err != nil {
				return fmt.Errorf("sincronizar arquivo temporário: %w", err)
			}
			if _, err := file.Seek(0, io.SeekStart); err != nil {
				return fmt.Errorf("reposicionar para leitura: %w", err)
			}
			if _, err := io.ReadFull(file, buffer); err != nil {
				return fmt.Errorf("ler arquivo temporário: %w", err)
			}
			ioSink.Add(uint64(buffer[0]))
		}
	}
}

// goroutineWork cria várias goroutines intencionalmente estacionadas no mesmo
// canal. O perfil de goroutines agrupa as stacks iguais e mostra onde esperam.
func goroutineWork(ctx context.Context, configured settings) error {
	release := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(configured.goroutines)
	for range configured.goroutines {
		go goroutineWorker(release, &workers)
	}

	<-ctx.Done()
	close(release)
	workers.Wait()
	return nil
}

func goroutineWorker(release <-chan struct{}, workers *sync.WaitGroup) {
	defer workers.Done()
	<-release
}

// memoryWork aloca blocos e mantém todas as referências vivas. Por isso o heap
// com inuse_space mostra memória retida mesmo depois de uma coleta de GC.
func memoryWork(ctx context.Context, configured settings) error {
	const chunkSize = 1024 * 1024
	retained := make([][]byte, 0, configured.memoryMB)
	for index := 0; index < configured.memoryMB; index++ {
		chunk := make([]byte, chunkSize)
		// Tocar cada página força o sistema operacional a materializar a memória.
		for offset := 0; offset < len(chunk); offset += 4096 {
			chunk[offset] = byte(index)
		}
		retained = append(retained, chunk)
	}
	runtime.GC()
	fmt.Printf("memória retida: aproximadamente %d MB\n", configured.memoryMB)

	<-ctx.Done()
	runtime.KeepAlive(retained)
	return nil
}

// allocsWork cria objetos temporários e mantém somente o último. O heap pode
// permanecer moderado, mas alloc_space registra todo o volume criado, inclusive
// os bytes de objetos cujo espaço o garbage collector tornou reutilizável.
func allocsWork(ctx context.Context, _ settings) error {
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			buffer := make([]byte, 64*1024)
			buffer[0] = 1
			allocationSink = buffer
		}
	}
}

// mutexWork faz várias goroutines disputarem o mesmo lock. Manter o lock
// durante o Sleep é propositalmente ruim e torna a contenção fácil de observar.
func mutexWork(ctx context.Context, _ settings) error {
	// pprof: mutex profiling é amostrado; 1 pede o registro de toda contenção.
	previousFraction := runtime.SetMutexProfileFraction(1)
	defer runtime.SetMutexProfileFraction(previousFraction)

	workerCount := max(4, runtime.GOMAXPROCS(0)*4)
	var mutex sync.Mutex
	var counter uint64
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for ctx.Err() == nil {
				mutex.Lock()
				counter++
				time.Sleep(time.Millisecond)
				mutex.Unlock()
			}
		}()
	}
	workers.Wait()
	runtime.KeepAlive(counter)
	return nil
}

// blockWork usa produtores mais rápidos que o consumidor de um canal sem
// buffer. O block profile registra o tempo em que os envios ficaram impedidos.
func blockWork(ctx context.Context, _ settings) error {
	// pprof: block profiling vem desabilitado; taxa 1 registra todo bloqueio.
	runtime.SetBlockProfileRate(1)
	defer runtime.SetBlockProfileRate(0)

	messages := make(chan int64)
	workerCount := max(4, runtime.GOMAXPROCS(0)*2)
	var workers sync.WaitGroup
	workers.Add(workerCount + 1)

	go func() {
		defer workers.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case value := <-messages:
				blockSink.Add(value)
				time.Sleep(2 * time.Millisecond)
			}
		}
	}()

	for producer := 0; producer < workerCount; producer++ {
		go func(value int64) {
			defer workers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case messages <- value:
				}
			}
		}(int64(producer + 1))
	}

	workers.Wait()
	return nil
}
