package health

import "testing"

// TestStateTransitions percorre startup e drain sem envolver HTTP.
func TestStateTransitions(t *testing.T) {
	state := NewState()
	if state.Ready() {
		t.Fatal("estado inicial deveria ser não pronto")
	}
	state.MarkReady()
	if !state.Ready() {
		t.Fatal("MarkReady deveria tornar a instância pronta")
	}
	state.MarkNotReady()
	if state.Ready() {
		t.Fatal("MarkNotReady deveria retirar a instância da prontidão")
	}
}
