// Package labconfig valida a configuração textual que não participa da
// compilação Go, transformando o TODO do NGINX em contrato executável.
package labconfig

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestNginxUsesLeastConnectionsAndAllExporters transforma a parte textual do
// exercício em contrato: algoritmo e três destinos precisam estar presentes.
func TestNginxUsesLeastConnectionsAndAllExporters(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("não foi possível localizar o teste")
	}
	configPath := filepath.Join(filepath.Dir(currentFile), "..", "..", "deploy", "nginx", "default.conf")
	contents, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	config := string(contents)
	if !strings.Contains(config, "least_conn;") {
		t.Fatal("upstream deveria usar least_conn")
	}
	for _, instance := range []string{"exporter-1", "exporter-2", "exporter-3"} {
		line := "server " + instance + ":8080 max_fails=1 fail_timeout=5s;"
		if !strings.Contains(config, line) {
			t.Fatalf("configuração não contém %q", line)
		}
	}
}
