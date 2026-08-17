// Package admission decide se uma nova exportação pode entrar no serviço agora.
package admission

import (
	"errors"
	"time"

	"golang.org/x/time/rate"
)

// Gate é o contrato mínimo necessário pelo Handler. O exercício implementa a
// política sem acoplar os testes HTTP ao relógio real de rate.Limiter.
type Gate interface {
	Allow() bool
	Tokens() float64
}

// TokenBucket guarda o limitador e um relógio. O relógio é uma função para que
// os testes possam avançar o tempo sem usar Sleep.
type TokenBucket struct {
	limiter *rate.Limiter
	now     func() time.Time
}

// NewTokenBucket deve validar a configuração e criar o token bucket descrito pelo
// contrato do exercício. A escolha da API correta da biblioteca faz parte do TODO.
func NewTokenBucket(ratePerSecond float64, burst int) (*TokenBucket, error) {
	return newTokenBucket(ratePerSecond, burst, time.Now)
}

// newTokenBucket recebe o relógio para que os testes controlem a passagem do
// tempo. A função pública usa time.Now; a regra de rate e burst deve ser igual.
func newTokenBucket(ratePerSecond float64, burst int, now func() time.Time) (*TokenBucket, error) {
	// TODO 1: construa um estado válido ou devolva um erro de configuração.
	return nil, errors.New("TODO: implemente o token bucket")
}

// Allow deve tentar consumir uma permissão agora, sem bloquear a goroutine. O
// retorno falso será traduzido pelo Handler para 429.
func (b *TokenBucket) Allow() bool {
	// TODO 2: implemente a decisão imediata definida pelo contrato de Gate.
	return false
}

// Tokens já está pronto porque é somente observação para /stats. As guardas
// evitam panic enquanto o construtor ainda estiver incompleto no exercício.
func (b *TokenBucket) Tokens() float64 {
	if b == nil || b.limiter == nil || b.now == nil {
		return 0
	}
	return b.limiter.TokensAt(b.now())
}
