// Package domain contém o modelo e as regras centrais do negócio de pedidos.
//
// Neste exemplo de DDD tático, Order é a raiz do agregado e controla as
// mudanças que precisam manter pedido, itens, total e estado consistentes.
// Item é um valor interno desse agregado: ele não é carregado nem alterado
// isoladamente pela aplicação.
//
// O pacote não conhece HTTP, JSON, SQL, pgx ou PostgreSQL. Essa independência
// permite testar as regras do pedido apenas com objetos Go e garante que uma
// troca de tecnologia não altere a política do negócio.
package domain
