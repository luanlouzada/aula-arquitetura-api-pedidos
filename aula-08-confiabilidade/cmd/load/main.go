// Command load envia requisições sem retry automático. Assim, 202, 429 e 503
// permanecem visíveis no relatório para comparação das políticas do servidor.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"
)

// results agrega respostas de várias goroutines clientes. O mutex permite que
// somente uma goroutine por vez altere maps e slice, que não aceitam escrita
// concorrente com segurança.
type results struct {
	mutex           sync.Mutex
	status          map[int]int
	instances       map[string]int
	transportErrors int
	latencies       []time.Duration
}

// main cria um pool de clientes HTTP, controla o ritmo de envio e imprime uma
// visão externa. Ele mede a decisão HTTP, não o processamento após o 202.
func main() {
	url := flag.String("url", "http://127.0.0.1:8080/jobs", "endpoint de entrada, direto ou via NGINX")
	total := flag.Int("requests", 60, "quantidade total de requisições")
	concurrency := flag.Int("concurrency", 10, "quantidade de clientes simultâneos")
	ratePerSecond := flag.Int("rate", 0, "ritmo de envio em req/s; zero significa rajada")
	flag.Parse()
	if *total <= 0 || *concurrency <= 0 || *ratePerSecond < 0 {
		panic("requests e concurrency devem ser positivos; rate não pode ser negativo")
	}

	client := &http.Client{Timeout: 3 * time.Second}
	work := make(chan int)
	result := &results{status: make(map[int]int), instances: make(map[string]int)}
	var senders sync.WaitGroup
	startedAt := time.Now()

	for range *concurrency {
		senders.Add(1)
		go func() {
			defer senders.Done()
			for id := range work {
				send(client, *url, id+1, result)
			}
		}()
	}

	var ticker *time.Ticker
	if *ratePerSecond > 0 {
		ticker = time.NewTicker(time.Second / time.Duration(*ratePerSecond))
		defer ticker.Stop()
	}
	for id := range *total {
		if ticker != nil {
			<-ticker.C
		}
		work <- id
	}
	close(work)
	senders.Wait()
	elapsed := time.Since(startedAt)

	result.mutex.Lock()
	defer result.mutex.Unlock()
	sort.Slice(result.latencies, func(i, j int) bool { return result.latencies[i] < result.latencies[j] })
	fmt.Printf("requisições=%d concorrência=%d limite_envio=%d req/s duração=%s taxa_observada=%.1f req/s\n",
		*total, *concurrency, *ratePerSecond, elapsed.Round(time.Millisecond), float64(*total)/elapsed.Seconds())
	fmt.Printf("status: 202=%d 429=%d 503=%d outros=%d erros_transporte=%d\n",
		result.status[http.StatusAccepted],
		result.status[http.StatusTooManyRequests],
		result.status[http.StatusServiceUnavailable],
		countOther(result.status),
		result.transportErrors,
	)
	fmt.Printf("distribuição_por_instância=%v\n", result.instances)
	if len(result.latencies) > 0 {
		fmt.Printf("latência_http_p50=%s latência_http_p95=%s\n",
			percentile(result.latencies, 0.50).Round(time.Microsecond),
			percentile(result.latencies, 0.95).Round(time.Microsecond))
	}
}

// send executa uma tentativa sem retry. Manter uma tentativa por OrderID deixa
// 429 e 503 visíveis e evita introduzir duplicação no experimento.
func send(client *http.Client, url string, orderID int, result *results) {
	body := []byte(fmt.Sprintf(`{"order_id":%d}`, orderID))
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		result.recordTransportError()
		return
	}
	request.Header.Set("Content-Type", "application/json")
	startedAt := time.Now()
	response, err := client.Do(request)
	latency := time.Since(startedAt)
	if err != nil {
		result.recordTransportError()
		return
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	result.record(response.StatusCode, response.Header.Get("X-Instance-ID"), latency)
}

// record classifica uma resposta e usa X-Instance-ID para tornar a escolha do
// NGINX observável no relatório final.
func (r *results) record(status int, instance string, latency time.Duration) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.status[status]++
	if instance == "" {
		instance = "sem-cabeçalho"
	}
	r.instances[instance]++
	r.latencies = append(r.latencies, latency)
}

// recordTransportError separa falha de rede de uma resposta HTTP válida. Um 503
// é uma decisão do servidor; conexão recusada não possui status HTTP.
func (r *results) recordTransportError() {
	r.mutex.Lock()
	r.transportErrors++
	r.mutex.Unlock()
}

// countOther reúne respostas fora do contrato principal sem descartá-las.
func countOther(statuses map[int]int) int {
	total := 0
	for status, count := range statuses {
		if status != http.StatusAccepted && status != http.StatusTooManyRequests && status != http.StatusServiceUnavailable {
			total += count
		}
	}
	return total
}

// percentile escolhe uma posição de um slice já ordenado. A aproximação sem
// interpolação é suficiente para comparar os cenários deste laboratório.
func percentile(values []time.Duration, fraction float64) time.Duration {
	return values[int(float64(len(values)-1)*fraction)]
}
