// Command load cria um pedido grande e consulta esse mesmo pedido repetidamente.
// Ele existe para produzir uma carga reproduzível no laboratório de profiling;
// não faz parte da API entregue aos clientes.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type createOrderRequest struct {
	Customer string                   `json:"cliente"`
	Items    []createOrderItemRequest `json:"itens"`
}

type createOrderItemRequest struct {
	ProductName    string `json:"produto"`
	UnitPriceCents int64  `json:"preco_unitario_centavos"`
	Quantity       int    `json:"quantidade"`
}

type createOrderResponse struct {
	ID int64 `json:"id"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	baseURL := flag.String("base-url", "http://127.0.0.1:8083", "endereço da API")
	itemCount := flag.Int("items", 400, "quantidade de itens do pedido usado na carga")
	concurrency := flag.Int("concurrency", 4, "quantidade de consultas simultâneas")
	duration := flag.Duration("duration", 35*time.Second, "duração das consultas")
	flag.Parse()

	if *itemCount <= 0 || *concurrency <= 0 || *duration <= 0 {
		return errors.New("items, concurrency e duration devem ser maiores que zero")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	orderID, payloadBytes, err := createLargeOrder(client, *baseURL, *itemCount)
	if err != nil {
		return err
	}

	fmt.Printf(
		"pedido %d criado com %d itens; payload=%d bytes\n",
		orderID,
		*itemCount,
		payloadBytes,
	)
	fmt.Printf(
		"consultando GET /orders/%d por %s com concorrência %d\n",
		orderID,
		duration.String(),
		*concurrency,
	)

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	var successes atomic.Int64
	var failures atomic.Int64
	var workers sync.WaitGroup
	workers.Add(*concurrency)
	for range *concurrency {
		go func() {
			defer workers.Done()
			loadOrder(ctx, client, *baseURL, orderID, &successes, &failures)
		}()
	}
	workers.Wait()

	fmt.Printf(
		"carga concluída: sucessos=%d falhas=%d\n",
		successes.Load(),
		failures.Load(),
	)
	if failures.Load() > 0 {
		return errors.New("uma ou mais consultas falharam")
	}
	return nil
}

// createLargeOrder usa POST /orders, portanto o conjunto de dados atravessa o
// mesmo Controller, Service, domínio e Repository usado por qualquer cliente.
func createLargeOrder(client *http.Client, baseURL string, itemCount int) (int64, int, error) {
	input := createOrderRequest{
		Customer: "Cliente do profiling",
		Items:    make([]createOrderItemRequest, itemCount),
	}
	for index := range input.Items {
		input.Items[index] = createOrderItemRequest{
			ProductName:    fmt.Sprintf("Produto %04d", index),
			UnitPriceCents: int64(100 + index),
			Quantity:       2,
		}
	}

	payload, err := json.Marshal(input)
	if err != nil {
		return 0, 0, fmt.Errorf("serializar pedido da carga: %w", err)
	}
	request, err := http.NewRequest(
		http.MethodPost,
		baseURL+"/orders",
		bytes.NewReader(payload),
	)
	if err != nil {
		return 0, 0, fmt.Errorf("montar criação do pedido: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		return 0, 0, fmt.Errorf("criar pedido da carga: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4*1024))
		return 0, 0, fmt.Errorf(
			"POST /orders status = %d; body = %s",
			response.StatusCode,
			bytes.TrimSpace(body),
		)
	}

	var output createOrderResponse
	if err := json.NewDecoder(response.Body).Decode(&output); err != nil {
		return 0, 0, fmt.Errorf("ler pedido criado: %w", err)
	}
	if output.ID <= 0 {
		return 0, 0, errors.New("POST /orders não devolveu um id válido")
	}
	return output.ID, len(payload), nil
}

// loadOrder descarta o corpo somente depois de lê-lo por completo. Isso inclui
// mapeamento e serialização na medição e permite que o cliente reutilize a conexão.
func loadOrder(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	orderID int64,
	successes *atomic.Int64,
	failures *atomic.Int64,
) {
	url := fmt.Sprintf("%s/orders/%d", baseURL, orderID)
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			failures.Add(1)
			return
		}

		response, err := client.Do(request)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			failures.Add(1)
			continue
		}
		_, readErr := io.Copy(io.Discard, response.Body)
		closeErr := response.Body.Close()
		if response.StatusCode != http.StatusOK || readErr != nil || closeErr != nil {
			failures.Add(1)
			continue
		}
		successes.Add(1)
	}
}
