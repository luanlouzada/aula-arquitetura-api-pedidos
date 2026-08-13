// Command cachelab cria um pedido e compara uma leitura fria com leituras
// quentes da mesma chave. Ele produz carga para a aula; não faz parte da API.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"time"
)

type settings struct {
	apiURL       string
	items        int
	warmRequests int
}

type createOrderRequest struct {
	Customer string                   `json:"cliente"`
	Items    []createOrderItemRequest `json:"itens"`
}

type createOrderItemRequest struct {
	ProductName    string `json:"produto"`
	UnitPriceCents int64  `json:"preco_unitario_centavos"`
	Quantity       int    `json:"quantidade"`
}

type orderResponse struct {
	ID int64 `json:"id"`
}

type metricsResponse struct {
	Counters struct {
		Hits     uint64  `json:"hits"`
		Misses   uint64  `json:"misses"`
		Errors   uint64  `json:"errors"`
		HitRatio float64 `json:"hit_ratio"`
	} `json:"counters"`
}

func main() {
	configuration := parseFlags()
	client := &http.Client{Timeout: 10 * time.Second}

	before, err := readMetrics(client, configuration.apiURL)
	if err != nil {
		panic(err)
	}
	orderID, err := createOrder(client, configuration)
	if err != nil {
		panic(err)
	}

	cold, err := measureGet(client, configuration.apiURL, orderID)
	if err != nil {
		panic(err)
	}
	warm := make([]time.Duration, 0, configuration.warmRequests)
	for range configuration.warmRequests {
		duration, err := measureGet(client, configuration.apiURL, orderID)
		if err != nil {
			panic(err)
		}
		warm = append(warm, duration)
	}
	after, err := readMetrics(client, configuration.apiURL)
	if err != nil {
		panic(err)
	}

	hits := after.Counters.Hits - before.Counters.Hits
	misses := after.Counters.Misses - before.Counters.Misses
	errors := after.Counters.Errors - before.Counters.Errors
	reads := hits + misses
	ratio := 0.0
	if reads > 0 {
		ratio = float64(hits) / float64(reads)
	}

	fmt.Printf("pedido %d criado com %d itens\n", orderID, configuration.items)
	if reads == 0 {
		fmt.Printf("primeira leitura: %s (SEM CACHE: PostgreSQL)\n", cold)
		fmt.Printf(
			"leituras repetidas: %d média=%s p50=%s p95=%s (SEM CACHE: PostgreSQL em todas)\n",
			len(warm), average(warm), percentile(warm, 0.50), percentile(warm, 0.95),
		)
	} else {
		fmt.Printf("primeira leitura: %s (MISS: Redis → PostgreSQL → SET)\n", cold)
		fmt.Printf(
			"leituras repetidas: %d média=%s p50=%s p95=%s (HIT: Redis)\n",
			len(warm), average(warm), percentile(warm, 0.50), percentile(warm, 0.95),
		)
	}
	fmt.Printf(
		"métricas desta execução: hits=%d misses=%d errors=%d hit_ratio=%.2f%%\n",
		hits, misses, errors, ratio*100,
	)
}

func parseFlags() settings {
	var configuration settings
	flag.StringVar(&configuration.apiURL, "api-url", "http://127.0.0.1:8084", "endereço base da API")
	flag.IntVar(&configuration.items, "items", 100, "quantidade de itens no pedido")
	flag.IntVar(&configuration.warmRequests, "warm-requests", 50, "leituras feitas depois do primeiro miss")
	flag.Parse()
	if configuration.items <= 0 || configuration.warmRequests <= 0 {
		panic("items e warm-requests devem ser positivos")
	}
	return configuration
}

func createOrder(client *http.Client, configuration settings) (int64, error) {
	items := make([]createOrderItemRequest, configuration.items)
	for index := range items {
		items[index] = createOrderItemRequest{
			ProductName:    fmt.Sprintf("Produto %03d", index+1),
			UnitPriceCents: int64(1000 + index),
			Quantity:       1,
		}
	}
	payload, err := json.Marshal(createOrderRequest{Customer: "Cliente do cache lab", Items: items})
	if err != nil {
		return 0, err
	}
	request, err := http.NewRequest(http.MethodPost, configuration.apiURL+"/orders", bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return 0, fmt.Errorf("criar pedido: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(response.Body)
		return 0, fmt.Errorf("criar pedido: status %d: %s", response.StatusCode, body)
	}
	var order orderResponse
	if err := json.NewDecoder(response.Body).Decode(&order); err != nil {
		return 0, err
	}
	return order.ID, nil
}

func measureGet(client *http.Client, apiURL string, orderID int64) (time.Duration, error) {
	startedAt := time.Now()
	response, err := client.Get(fmt.Sprintf("%s/orders/%d", apiURL, orderID))
	duration := time.Since(startedAt)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("consultar pedido: status %d", response.StatusCode)
	}
	return duration, nil
}

func readMetrics(client *http.Client, apiURL string) (metricsResponse, error) {
	response, err := client.Get(apiURL + "/metrics/cache")
	if err != nil {
		return metricsResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return metricsResponse{}, fmt.Errorf("consultar métricas: status %d", response.StatusCode)
	}
	var metrics metricsResponse
	if err := json.NewDecoder(response.Body).Decode(&metrics); err != nil {
		return metricsResponse{}, err
	}
	return metrics, nil
}

func average(values []time.Duration) time.Duration {
	var total time.Duration
	for _, value := range values {
		total += value
	}
	return total / time.Duration(len(values))
}

func percentile(values []time.Duration, fraction float64) time.Duration {
	ordered := append([]time.Duration(nil), values...)
	sort.Slice(ordered, func(left, right int) bool { return ordered[left] < ordered[right] })
	index := int(math.Ceil(float64(len(ordered))*fraction)) - 1
	if index < 0 {
		index = 0
	}
	return ordered[index]
}
