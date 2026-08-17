// Package health mantém o estado operacional observado por liveness e
// readiness. Os dois sinais respondem a perguntas diferentes.
package health

import "sync/atomic"

// State começa não pronto. O processo só deve anunciar readiness depois que a
// montagem terminou e o servidor começou a escutar. Durante o shutdown ele
// volta a não pronto antes de fechar o HTTP.
type State struct {
	ready atomic.Bool
}

// NewState devolve uma instância inicialmente não pronta. O main chama
// MarkReady somente depois de montar e iniciar os componentes necessários.
func NewState() *State {
	return &State{}
}

// MarkReady permite que um balanceador envie novas requisições de negócio.
func (s *State) MarkReady() {
	s.ready.Store(true)
}

// MarkNotReady pede a retirada da instância da rotação. Isso não mata o
// processo: trabalho já aceito ainda pode ser concluído.
func (s *State) MarkNotReady() {
	s.ready.Store(false)
}

// Ready pode ser chamado por várias goroutines ao mesmo tempo sem data race.
func (s *State) Ready() bool {
	return s.ready.Load()
}
