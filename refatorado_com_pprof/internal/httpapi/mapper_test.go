package httpapi

import (
	"fmt"
	"testing"

	"aula-pedidos/refatorado_com_pprof/internal/domain"
)

// TestToOrderResponsePreservesTheHTTPContract protege o comportamento que a
// otimização não pode alterar: quantidade, nomes, subtotais e total do pedido.
func TestToOrderResponsePreservesTheHTTPContract(t *testing.T) {
	order := newOrderForMapper(t, 2)

	response := toOrderResponse(order)

	if got, want := len(response.Items), 2; got != want {
		t.Fatalf("len(response.Items) = %d, want %d", got, want)
	}
	if got, want := response.Items[1].ProductName, "Produto 0001"; got != want {
		t.Fatalf("response.Items[1].ProductName = %q, want %q", got, want)
	}
	if got, want := response.Items[1].SubtotalCents, int64(202); got != want {
		t.Fatalf("response.Items[1].SubtotalCents = %d, want %d", got, want)
	}
	if got, want := response.TotalCents, int64(402); got != want {
		t.Fatalf("response.TotalCents = %d, want %d", got, want)
	}
}

// BenchmarkToOrderResponse mede somente a conversão do agregado para o DTO
// HTTP. A criação do pedido fica fora de b.Loop e não entra nos ns/op ou B/op.
func BenchmarkToOrderResponse(b *testing.B) {
	order := newOrderForMapper(b, 500)
	b.ReportAllocs()

	for b.Loop() {
		toOrderResponse(order)
	}
}

// newOrderForMapper cria a mesma entrada para o teste e o benchmark. O helper
// usa construtores do domínio para não depender de campos privados.
func newOrderForMapper(tb testing.TB, itemCount int) domain.Order {
	tb.Helper()

	items := make([]domain.Item, itemCount)
	for index := range items {
		item, err := domain.NewItem(
			fmt.Sprintf("Produto %04d", index),
			int64(100+index),
			2,
		)
		if err != nil {
			tb.Fatalf("domain.NewItem() error = %v", err)
		}
		items[index] = item
	}

	order, err := domain.NewOrder("Cliente do benchmark", items)
	if err != nil {
		tb.Fatalf("domain.NewOrder() error = %v", err)
	}
	return order
}
