// Package health mantém a decisão operacional de prontidão da instância.
package health

import "sync/atomic"

// State possui um único booleano atômico porque handlers leem a prontidão ao
// mesmo tempo em que o fluxo de startup ou shutdown a altera.
type State struct {
	ready atomic.Bool
}

// NewState deve começar como not-ready. O main torna a instância pronta somente
// depois de montar as dependências e iniciar o servidor.
func NewState() *State {
	return &State{}
}

// MarkReady deve anunciar que a montagem terminou e trabalho novo pode entrar.
func (s *State) MarkReady() {
	// TODO 3: publique a transição para pronta de forma segura entre goroutines.
}

// MarkNotReady deve iniciar o drain sem encerrar imediatamente o processo.
func (s *State) MarkNotReady() {
	// TODO 4: publique a transição para não pronta.
}

// Ready deve devolver a fotografia usada por /readyz e POST /exports.
func (s *State) Ready() bool {
	// TODO 5: devolva o estado atual sem data race.
	return false
}
