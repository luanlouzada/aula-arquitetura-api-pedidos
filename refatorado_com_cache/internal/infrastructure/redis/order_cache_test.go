package redis

import "testing"

func TestOrderKeyIncludesNamespaceFormatVersionAndID(t *testing.T) {
	if got, want := orderKey(42), "pedidos:orders:v1:42"; got != want {
		t.Fatalf("orderKey(42) = %q, want %q", got, want)
	}
}
