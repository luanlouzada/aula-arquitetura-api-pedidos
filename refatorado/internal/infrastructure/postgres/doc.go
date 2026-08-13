// Package postgres implementa no PostgreSQL as necessidades de persistência
// declaradas pela camada de aplicação.
//
// Este pacote pode conhecer tabelas, transações, SQL e pgx. Ele converte dados
// persistidos para objetos do domínio, mas não cria regras de pedido: mesmo ao
// ler o banco, pede ao domínio que valide o estado reconstituído.
package postgres
