package config

import "testing"

// TestLoadRejectsInvalidRate garante falha de inicialização antes de construir
// um token bucket com taxa negativa, nula ou não finita.
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

// TestLoadReadsExerciseIdentityAndCapacity confirma que o novo domínio possui
// configuração independente da referência.
func TestLoadReadsExerciseIdentityAndCapacity(t *testing.T) {
	t.Setenv("INSTANCE_ID", "exporter-test")
	t.Setenv("WORKERS", "3")
	t.Setenv("QUEUE_CAPACITY", "7")
	settings, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if settings.InstanceID != "exporter-test" || settings.Workers != 3 || settings.QueueCapacity != 7 {
		t.Fatalf("configuração inesperada: %+v", settings)
	}
}
