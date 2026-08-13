package config_test

import (
	"testing"

	"aula-pedidos/refatorado_com_pprof/internal/config"
)

func TestLoadReadsPprofAddress(t *testing.T) {
	t.Setenv("PPROF_ADDR", "127.0.0.1:7070")

	settings := config.Load()

	if got, want := settings.PprofAddress, "127.0.0.1:7070"; got != want {
		t.Fatalf("PprofAddress = %q, want %q", got, want)
	}
}

func TestLoadUsesDefaultWhenPprofAddressIsEmpty(t *testing.T) {
	t.Setenv("PPROF_ADDR", "")

	settings := config.Load()

	if got, want := settings.PprofAddress, "127.0.0.1:6060"; got != want {
		t.Fatalf("PprofAddress = %q, want %q", got, want)
	}
}
