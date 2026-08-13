package contract_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

type orderResponse struct {
	ID         int64               `json:"id"`
	Customer   string              `json:"cliente"`
	Status     string              `json:"status"`
	TotalCents int64               `json:"total_centavos"`
	Version    int                 `json:"versao"`
	CreatedAt  time.Time           `json:"criado_em"`
	Items      []orderItemResponse `json:"itens"`
}

type orderItemResponse struct {
	ProductName    string `json:"produto"`
	UnitPriceCents int64  `json:"preco_unitario_centavos"`
	Quantity       int    `json:"quantidade"`
	SubtotalCents  int64  `json:"subtotal_centavos"`
}

func TestAPIContract(t *testing.T) {
	baseURL := strings.TrimRight(os.Getenv("API_BASE_URL"), "/")
	if baseURL == "" {
		t.Skip("set API_BASE_URL to run the shared API contract")
	}

	invalid := requestJSON(t, http.MethodPost, baseURL+"/orders", map[string]any{
		"cliente": "",
		"itens":   []any{},
	})
	requireStatus(t, invalid, http.StatusUnprocessableEntity)
	invalid.Body.Close()

	created := createOrder(t, baseURL, "Ana")
	if created.ID <= 0 {
		t.Fatalf("created id = %d, want a positive value", created.ID)
	}
	if got, want := created.Status, "PENDENTE"; got != want {
		t.Fatalf("created status = %q, want %q", got, want)
	}
	if got, want := created.TotalCents, int64(900_000); got != want {
		t.Fatalf("created total = %d, want %d", got, want)
	}
	if got, want := created.Version, 1; got != want {
		t.Fatalf("created version = %d, want %d", got, want)
	}

	getResponse := requestJSON(t, http.MethodGet, fmt.Sprintf("%s/orders/%d", baseURL, created.ID), nil)
	requireStatus(t, getResponse, http.StatusOK)
	gotOrder := decodeOrder(t, getResponse)
	if got, want := gotOrder.Customer, "Ana"; got != want {
		t.Fatalf("GET customer = %q, want %q", got, want)
	}

	payResponse := requestJSON(t, http.MethodPatch, fmt.Sprintf("%s/orders/%d/pay", baseURL, created.ID), nil)
	requireStatus(t, payResponse, http.StatusOK)
	paid := decodeOrder(t, payResponse)
	if got, want := paid.Status, "PAGO"; got != want {
		t.Fatalf("paid status = %q, want %q", got, want)
	}
	if got, want := paid.Version, 2; got != want {
		t.Fatalf("paid version = %d, want %d", got, want)
	}

	cancelPaid := requestJSON(t, http.MethodPatch, fmt.Sprintf("%s/orders/%d/cancel", baseURL, created.ID), nil)
	requireStatus(t, cancelPaid, http.StatusConflict)
	cancelPaid.Body.Close()

	second := createOrder(t, baseURL, "Bruno")
	cancelResponse := requestJSON(t, http.MethodPatch, fmt.Sprintf("%s/orders/%d/cancel", baseURL, second.ID), nil)
	requireStatus(t, cancelResponse, http.StatusOK)
	canceled := decodeOrder(t, cancelResponse)
	if got, want := canceled.Status, "CANCELADO"; got != want {
		t.Fatalf("canceled status = %q, want %q", got, want)
	}

	invalidID := requestJSON(t, http.MethodGet, baseURL+"/orders/not-a-number", nil)
	requireStatus(t, invalidID, http.StatusBadRequest)
	invalidID.Body.Close()
}

func createOrder(t *testing.T, baseURL, customer string) orderResponse {
	t.Helper()
	response := requestJSON(t, http.MethodPost, baseURL+"/orders", map[string]any{
		"cliente": customer,
		"itens": []map[string]any{
			{
				"produto":                 "Notebook",
				"preco_unitario_centavos": 450_000,
				"quantidade":              2,
			},
		},
	})
	requireStatus(t, response, http.StatusCreated)
	return decodeOrder(t, response)
}

func requestJSON(t *testing.T, method, url string, value any) *http.Response {
	t.Helper()
	var body io.Reader = http.NoBody
	if value != nil {
		payload, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		body = bytes.NewReader(payload)
	}

	request, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	if value != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return response
}

func requireStatus(t *testing.T, response *http.Response, want int) {
	t.Helper()
	if response.StatusCode == want {
		return
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, want, body)
}

func decodeOrder(t *testing.T, response *http.Response) orderResponse {
	t.Helper()
	defer response.Body.Close()
	var order orderResponse
	if err := json.NewDecoder(response.Body).Decode(&order); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return order
}
