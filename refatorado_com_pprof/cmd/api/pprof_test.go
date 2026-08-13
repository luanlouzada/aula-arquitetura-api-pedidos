package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// pprof: TestPprofHandlerIsAvailable confirma que o servidor administrativo
// possui a página inicial sem precisar iniciar uma porta TCP durante o teste.
func TestPprofHandlerIsAvailable(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	response := httptest.NewRecorder()

	newPprofHandler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("GET /debug/pprof/ status = %d, want %d", response.Code, http.StatusOK)
	}
	if !strings.Contains(response.Body.String(), "Types of profiles available") {
		t.Fatalf("GET /debug/pprof/ did not return the pprof index")
	}
}

// pprof: TestPprofHandlerServesNamedProfile prova que o padrão terminado em
// barra e pprof.Index também atendem os perfis registrados no runtime.
func TestPprofHandlerServesNamedProfile(t *testing.T) {
	request := httptest.NewRequest(
		http.MethodGet,
		"/debug/pprof/goroutine?debug=1",
		nil,
	)
	response := httptest.NewRecorder()

	newPprofHandler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf(
			"GET /debug/pprof/goroutine status = %d, want %d",
			response.Code,
			http.StatusOK,
		)
	}
	if !strings.Contains(response.Body.String(), "goroutine profile") {
		t.Fatalf("GET /debug/pprof/goroutine did not return a goroutine profile")
	}
}

// pprof: TestPprofHandlerDoesNotExposeOrders prova que o mux administrativo
// não compartilha as rotas de negócio da API pública.
func TestPprofHandlerDoesNotExposeOrders(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/orders/1", nil)
	response := httptest.NewRecorder()

	newPprofHandler().ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("GET /orders/1 status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

// pprof: TestPprofHandlerRejectsPost confirma a exigência de GET documentada por
// net/http/pprof desde Go 1.22.
func TestPprofHandlerRejectsPost(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/debug/pprof/symbol", nil)
	response := httptest.NewRecorder()

	newPprofHandler().ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf(
			"POST /debug/pprof/symbol status = %d, want %d",
			response.Code,
			http.StatusMethodNotAllowed,
		)
	}
}
