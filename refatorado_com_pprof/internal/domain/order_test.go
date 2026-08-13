package domain_test

import (
	"errors"
	"testing"

	"aula-pedidos/refatorado_com_pprof/internal/domain"
)

// Estes testes usam package domain_test para exercitar apenas a API pública do
// domínio. As invariantes são verificadas sem acessar campos privados, HTTP ou
// PostgreSQL.

// TestNewOrderProtectsInvariantsAndCalculatesTotal cobre o caminho válido: os
// construtores normalizam nomes, calculam o total e definem o estado inicial.
func TestNewOrderProtectsInvariantsAndCalculatesTotal(t *testing.T) {
	item, err := domain.NewItem("  Notebook  ", 450_000, 2)
	if err != nil {
		t.Fatalf("NewItem() error = %v", err)
	}

	order, err := domain.NewOrder("  Ana  ", []domain.Item{item})
	if err != nil {
		t.Fatalf("NewOrder() error = %v", err)
	}

	if got, want := order.Customer(), "Ana"; got != want {
		t.Fatalf("Customer() = %q, want %q", got, want)
	}
	if got, want := order.TotalCents(), int64(900_000); got != want {
		t.Fatalf("TotalCents() = %d, want %d", got, want)
	}
	if got, want := order.Status(), domain.StatusPending; got != want {
		t.Fatalf("Status() = %q, want %q", got, want)
	}
	if got, want := order.Items()[0].ProductName(), "Notebook"; got != want {
		t.Fatalf("ProductName() = %q, want %q", got, want)
	}
}

// TestOrderRejectsInvalidInput documenta exemplos de estados que nunca devem
// chegar a existir como Item ou Order.
func TestOrderRejectsInvalidInput(t *testing.T) {
	validItem, err := domain.NewItem("Notebook", 450_000, 1)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		run  func() error
		want error
	}{
		{
			name: "customer required",
			run: func() error {
				_, err := domain.NewOrder("   ", []domain.Item{validItem})
				return err
			},
			want: domain.ErrCustomerRequired,
		},
		{
			name: "at least one item",
			run: func() error {
				_, err := domain.NewOrder("Ana", nil)
				return err
			},
			want: domain.ErrOrderWithoutItems,
		},
		{
			name: "positive quantity",
			run: func() error {
				_, err := domain.NewItem("Notebook", 450_000, 0)
				return err
			},
			want: domain.ErrInvalidItem,
		},
		{
			name: "positive price",
			run: func() error {
				_, err := domain.NewItem("Notebook", 0, 1)
				return err
			},
			want: domain.ErrInvalidItem,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

// TestOrderAllowsOnlyOneFinalTransition demonstra a máquina de estados: depois
// de PENDENTE → PAGO, a transição para CANCELADO precisa ser recusada.
func TestOrderAllowsOnlyOneFinalTransition(t *testing.T) {
	item, err := domain.NewItem("Notebook", 450_000, 1)
	if err != nil {
		t.Fatal(err)
	}
	order, err := domain.NewOrder("Ana", []domain.Item{item})
	if err != nil {
		t.Fatal(err)
	}

	if err := order.Pay(); err != nil {
		t.Fatalf("Pay() error = %v", err)
	}
	if got, want := order.Status(), domain.StatusPaid; got != want {
		t.Fatalf("Status() = %q, want %q", got, want)
	}
	if err := order.Cancel(); !errors.Is(err, domain.ErrInvalidStatusTransition) {
		t.Fatalf("Cancel() error = %v, want %v", err, domain.ErrInvalidStatusTransition)
	}
}
