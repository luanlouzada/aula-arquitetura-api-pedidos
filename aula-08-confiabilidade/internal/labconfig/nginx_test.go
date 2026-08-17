// Package labconfig valida arquivos que participam do laboratório, mas não são
// compilados pelo Go. Assim, uma alteração acidental no NGINX também quebra a
// suíte antes da demonstração em sala.
package labconfig

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestNginxUsesLeastConnectionsAndAllInstances exige o algoritmo explicado no
// README e os três destinos que o Compose cria.
func TestNginxUsesLeastConnectionsAndAllInstances(t *testing.T) {
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
	for _, instance := range []string{"api-1", "api-2", "api-3"} {
		line := "server " + instance + ":8080 max_fails=1 fail_timeout=5s;"
		if !strings.Contains(config, line) {
			t.Fatalf("configuração não contém %q", line)
		}
	}
}
