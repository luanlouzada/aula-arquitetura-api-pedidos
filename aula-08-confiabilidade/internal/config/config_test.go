package config

import "testing"

// TestLoadRejectsInvalidRate confirma a validação antes da montagem do servidor,
// inclusive para valores especiais que strconv.ParseFloat aceita.
func TestLoadRejectsInvalidRate(t *testing.T) {
	for _, value := range []string{"0", "-1", "NaN", "+Inf", "-Inf"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("RATE_LIMIT_RPS", value)
			if _, err := Load(); err == nil {
				t.Fatalf("RATE_LIMIT_RPS=%s deveria ser rejeitado", value)
			}
		})
	}
}

// TestLoadReadsInstanceAndCapacity prova que valores explícitos substituem os
// padrões sem depender do ambiente externo ao teste.
func TestLoadReadsInstanceAndCapacity(t *testing.T) {
	t.Setenv("INSTANCE_ID", "api-test")
	t.Setenv("WORKERS", "3")
	t.Setenv("QUEUE_CAPACITY", "7")
	settings, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if settings.InstanceID != "api-test" || settings.Workers != 3 || settings.QueueCapacity != 7 {
		t.Fatalf("configuração inesperada: %+v", settings)
	}
}
