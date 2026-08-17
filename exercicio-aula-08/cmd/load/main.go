// Command load envia solicitações de exportação sem retry automático.
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

// results agrega respostas produzidas por vários clientes concorrentes. O mutex
// permite que somente uma goroutine por vez altere maps e slice, que não são
// seguros para escrita simultânea.
type results struct {
	mutex           sync.Mutex
	status          map[int]int
	instances       map[string]int
	transportErrors int
	latencies       []time.Duration
}

// main cria clientes concorrentes, opcionalmente limita o ritmo de envio e
// resume o que um consumidor externo realmente observou.
func main() {
	url := flag.String("url", "http://127.0.0.1:8090/exports", "endpoint direto ou via NGINX")
	total := flag.Int("requests", 60, "quantidade total de solicitações")
	concurrency := flag.Int("concurrency", 10, "clientes simultâneos")
	ratePerSecond := flag.Int("rate", 0, "ritmo em req/s; zero significa rajada")
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
				send(client, *url, id, result)
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
	fmt.Printf("requisições=%d concorrência=%d duração=%s taxa_observada=%.1f req/s\n",
		*total, *concurrency, elapsed.Round(time.Millisecond), float64(*total)/elapsed.Seconds())
	fmt.Printf("status: 202=%d 429=%d 503=%d outros=%d erros_transporte=%d\n",
		result.status[202], result.status[429], result.status[503], countOther(result.status), result.transportErrors)
	fmt.Printf("distribuição_por_instância=%v\n", result.instances)
	if len(result.latencies) > 0 {
		fmt.Printf("latência_http_p50=%s latência_http_p95=%s\n",
			percentile(result.latencies, .50).Round(time.Microsecond),
			percentile(result.latencies, .95).Round(time.Microsecond))
	}
}

// send faz exatamente uma tentativa e não repete 429 ou 503. Isso preserva as
// respostas no relatório e evita duplicar exportações.
func send(client *http.Client, url string, id int, result *results) {
	body := []byte(fmt.Sprintf(`{"report":"sales-%03d"}`, id+1))
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

// record classifica status, latência e a instância escolhida pelo NGINX.
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

// recordTransportError conta falhas sem resposta HTTP separadamente de 503.
func (r *results) recordTransportError() {
	r.mutex.Lock()
	r.transportErrors++
	r.mutex.Unlock()
}

// countOther preserva visibilidade de qualquer resposta fora de 202, 429 e 503.
func countOther(statuses map[int]int) int {
	total := 0
	for status, count := range statuses {
		if status != 202 && status != 429 && status != 503 {
			total += count
		}
	}
	return total
}

// percentile recebe durações já ordenadas e escolhe a posição aproximada. Não
// há interpolação porque o objetivo é comparar cenários, não produzir uma
// métrica formal de nível de serviço.
func percentile(values []time.Duration, fraction float64) time.Duration {
	return values[int(float64(len(values)-1)*fraction)]
}
