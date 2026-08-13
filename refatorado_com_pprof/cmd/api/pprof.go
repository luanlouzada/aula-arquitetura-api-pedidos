package main

import (
	"net/http"
	// pprof: este pacote da biblioteca padrão liga endpoints HTTP aos perfis
	// mantidos pelo runtime do Go; não existe dependência externa no go.mod.
	httppprof "net/http/pprof"
)

// newPprofHandler monta a interface administrativa de profiling sem usar o
// DefaultServeMux global. Assim somente o servidor de diagnóstico conhece os
// endpoints /debug/pprof; a API pública continua expondo apenas /orders.
func newPprofHandler() http.Handler {
	mux := http.NewServeMux()

	// pprof: Index apresenta a página HTML e atende perfis nomeados, como heap,
	// allocs e goroutine. Mutex e block só acumulam dados se suas taxas forem
	// habilitadas com funções do runtime; esta API não as habilita.
	mux.HandleFunc("GET /debug/pprof/", httppprof.Index)
	// pprof: Cmdline informa o comando que iniciou o processo.
	mux.HandleFunc("GET /debug/pprof/cmdline", httppprof.Cmdline)
	// pprof: Profile coleta amostras de CPU durante o intervalo solicitado.
	mux.HandleFunc("GET /debug/pprof/profile", httppprof.Profile)
	// pprof: Symbol traduz endereços do programa para nomes de funções.
	mux.HandleFunc("GET /debug/pprof/symbol", httppprof.Symbol)
	// pprof: Trace coleta eventos do scheduler, syscalls, GC e goroutines.
	mux.HandleFunc("GET /debug/pprof/trace", httppprof.Trace)

	return mux
}
