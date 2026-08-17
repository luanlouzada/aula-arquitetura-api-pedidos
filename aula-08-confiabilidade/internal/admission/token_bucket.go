// Package admission decide se uma nova requisição pode entrar no serviço agora.
// Essa decisão acontece antes da fila e dos workers para que a sobrecarga seja
// recusada cedo, enquanto ainda custa pouco para o servidor.
package admission

import (
	"fmt"
	"math"
	"time"

	"golang.org/x/time/rate"
)

// Gate é o contrato mínimo usado pela camada HTTP. Allow não espera por uma
// permissão futura: ele responde imediatamente se a requisição pode prosseguir.
type Gate interface {
	Allow() bool
	Tokens() float64
}

// TokenBucket envolve o limitador concorrente de golang.org/x/time/rate.
//
// RatePerSecond é a velocidade de reposição. Burst é o máximo de permissões
// acumuladas. Cada Allow bem-sucedido consome uma permissão. O nome "token" não
// representa login nem autenticação; aqui ele significa somente uma unidade de
// permissão para admitir uma operação.
type TokenBucket struct {
	limiter *rate.Limiter
	now     func() time.Time
}

// NewTokenBucket cria um balde inicialmente cheio. Por exemplo, taxa 2 e burst
// 3 permitem uma rajada imediata de três operações e depois repõem duas
// permissões por segundo, sem deixar o saldo ultrapassar três.
func NewTokenBucket(ratePerSecond float64, burst int) (*TokenBucket, error) {
	return newTokenBucket(ratePerSecond, burst, time.Now)
}

// newTokenBucket recebe o relógio como dependência para que o teste avance o
// tempo de modo determinístico. A aplicação pública usa time.Now.
func newTokenBucket(ratePerSecond float64, burst int, now func() time.Time) (*TokenBucket, error) {
	if ratePerSecond <= 0 || math.IsNaN(ratePerSecond) || math.IsInf(ratePerSecond, 0) {
		return nil, fmt.Errorf("ratePerSecond deve ser positivo e finito: %v", ratePerSecond)
	}
	if burst <= 0 {
		return nil, fmt.Errorf("burst deve ser positivo: %d", burst)
	}
	if now == nil {
		return nil, fmt.Errorf("relógio é obrigatório")
	}
	return &TokenBucket{
		limiter: rate.NewLimiter(rate.Limit(ratePerSecond), burst),
		now:     now,
	}, nil
}

// Allow tenta consumir uma permissão no instante atual. AllowN é usado em vez
// de Wait porque uma API sob sobrecarga deve responder 429 rapidamente, não
// manter mais conexões e goroutines esperando dentro do processo.
func (b *TokenBucket) Allow() bool {
	return b.limiter.AllowN(b.now(), 1)
}

// Tokens devolve uma fotografia aproximada do saldo disponível. O número pode
// ter parte decimal porque a reposição ocorre continuamente ao longo do tempo.
func (b *TokenBucket) Tokens() float64 {
	return b.limiter.TokensAt(b.now())
}
