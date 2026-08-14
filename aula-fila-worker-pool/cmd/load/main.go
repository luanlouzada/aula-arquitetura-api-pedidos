// Command load envia uma rajada concorrente para o laboratório e resume os
// resultados. Ele evita depender de uma ferramenta externa de benchmark.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// main executa um cliente de carga separado da API. A concorrência configurada
// aqui controla quantas goroutines enviam HTTP ao mesmo tempo; ela não altera a
// quantidade de workers que processa Jobs no servidor.
func main() {
	url := flag.String("url", "http://127.0.0.1:8087/jobs", "endpoint do laboratório")
	total := flag.Int("requests", 100, "quantidade total de requisições")
	concurrency := flag.Int("concurrency", 20, "requisições simultâneas")
	rate := flag.Int("rate", 0, "ritmo máximo de envio em req/s; zero envia uma rajada")
	flag.Parse()
	if *total <= 0 || *concurrency <= 0 || *rate < 0 {
		panic("requests e concurrency devem ser positivos; rate não pode ser negativo")
	}

	client := &http.Client{Timeout: 3 * time.Second}
	// work distribui números de requisição entre as goroutines deste cliente. Ele
	// não é a fila de Jobs da API. latencies recebe um resultado por tentativa e
	// usa buffer suficiente para que os remetentes não esperem pelo relatório.
	work := make(chan int)
	latencies := make(chan time.Duration, *total)
	// Várias goroutines atualizam os resultados. Os contadores atômicos evitam
	// data race sem exigir um mutex em torno de cada resposta.
	var accepted atomic.Uint64
	var rejected atomic.Uint64
	var other atomic.Uint64
	var transportErrors atomic.Uint64
	var workers sync.WaitGroup

	startedAt := time.Now()
	// Este é um pool de remetentes HTTP. Cada goroutine recebe IDs de work e envia
	// uma requisição por vez até que o channel seja fechado.
	for workerID := 0; workerID < *concurrency; workerID++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for id := range work {
				body := []byte(fmt.Sprintf(`{"order_id":%d}`, id+1))
				request, err := http.NewRequest(http.MethodPost, *url, bytes.NewReader(body))
				if err != nil {
					transportErrors.Add(1)
					continue
				}
				request.Header.Set("Content-Type", "application/json")
				requestStarted := time.Now()
				response, err := client.Do(request)
				// Esta é a latência para a API aceitar ou rejeitar a requisição. Ela
				// não inclui o processamento assíncrono executado depois do 202.
				latencies <- time.Since(requestStarted)
				if err != nil {
					transportErrors.Add(1)
					continue
				}
				// Consumir e fechar o corpo permite que net/http reutilize a conexão
				// TCP nas próximas requisições do teste.
				_, _ = io.Copy(io.Discard, response.Body)
				_ = response.Body.Close()
				switch response.StatusCode {
				case http.StatusAccepted:
					accepted.Add(1)
				case http.StatusServiceUnavailable:
					rejected.Add(1)
				default:
					other.Add(1)
				}
			}
		}()
	}
	// Sem rate, os IDs são enviados tão rápido quanto as goroutines conseguem
	// consumir: uma rajada. Com rate, o ticker libera no máximo um novo ID por
	// intervalo e cria uma taxa de chegada sustentada.
	var ticker *time.Ticker
	if *rate > 0 {
		ticker = time.NewTicker(time.Second / time.Duration(*rate))
		defer ticker.Stop()
	}
	for id := 0; id < *total; id++ {
		if ticker != nil {
			<-ticker.C
		}
		work <- id
	}
	// Depois do último ID, close avisa aos remetentes que não existe mais trabalho.
	// Wait garante que todas as respostas foram classificadas antes do relatório.
	close(work)
	workers.Wait()
	close(latencies)
	elapsed := time.Since(startedAt)

	allLatencies := make([]time.Duration, 0, *total)
	for latency := range latencies {
		allLatencies = append(allLatencies, latency)
	}
	sort.Slice(allLatencies, func(i, j int) bool { return allLatencies[i] < allLatencies[j] })

	fmt.Printf("requisições=%d concorrência=%d limite_envio=%d req/s duração=%s taxa_observada=%.1f req/s\n",
		*total,
		*concurrency,
		*rate,
		elapsed.Round(time.Millisecond),
		float64(*total)/elapsed.Seconds(),
	)
	fmt.Printf("aceitas_202=%d rejeitadas_503=%d outros=%d erros_transporte=%d\n",
		accepted.Load(), rejected.Load(), other.Load(), transportErrors.Load())
	if len(allLatencies) > 0 {
		fmt.Printf("latência_http_p50=%s latência_http_p95=%s\n",
			percentile(allLatencies, 0.50).Round(time.Microsecond),
			percentile(allLatencies, 0.95).Round(time.Microsecond),
		)
	}
}

// percentile recebe valores já ordenados e escolhe a posição correspondente à
// fração solicitada. O cálculo é intencionalmente simples e não interpola entre
// duas posições; ele é suficiente para comparar os cenários do laboratório.
func percentile(values []time.Duration, fraction float64) time.Duration {
	index := int(float64(len(values)-1) * fraction)
	return values[index]
}
