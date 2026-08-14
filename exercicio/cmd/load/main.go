// Command load envia uma onda de comandas para a cozinha e resume os
// resultados. A concorrência daqui controla quantos clientes pedem ao mesmo
// tempo; ela não altera a quantidade de cozinheiros no servidor.
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

var menu = []string{"lasanha", "pizza", "risotto", "hamburguer", "salada"}

func main() {
	url := flag.String("url", "http://127.0.0.1:8088/tickets", "endpoint da cozinha")
	total := flag.Int("requests", 100, "quantidade total de requisições")
	concurrency := flag.Int("concurrency", 20, "requisições simultâneas")
	rate := flag.Int("rate", 0, "ritmo máximo de envio em req/s; zero envia uma rajada")
	flag.Parse()
	if *total <= 0 || *concurrency <= 0 || *rate < 0 {
		panic("requests e concurrency devem ser positivos; rate não pode ser negativo")
	}

	client := &http.Client{Timeout: 3 * time.Second}
	work := make(chan int)
	latencies := make(chan time.Duration, *total)
	var accepted atomic.Uint64
	var rejected atomic.Uint64
	var other atomic.Uint64
	var transportErrors atomic.Uint64
	var workers sync.WaitGroup

	startedAt := time.Now()
	for i := 0; i < *concurrency; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for id := range work {
				dish := menu[id%len(menu)]
				body := []byte(fmt.Sprintf(`{"prato":%q}`, dish))
				request, err := http.NewRequest(http.MethodPost, *url, bytes.NewReader(body))
				if err != nil {
					transportErrors.Add(1)
					continue
				}
				request.Header.Set("Content-Type", "application/json")
				requestStarted := time.Now()
				response, err := client.Do(request)
				latencies <- time.Since(requestStarted)
				if err != nil {
					transportErrors.Add(1)
					continue
				}
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

func percentile(values []time.Duration, fraction float64) time.Duration {
	index := int(float64(len(values)-1) * fraction)
	return values[index]
}
