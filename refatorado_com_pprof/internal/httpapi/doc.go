// Package httpapi adapta o protocolo HTTP aos casos de uso da aplicação.
//
// Aqui ficam decisões de protocolo e representação: rotas, leitura de JSON,
// parâmetros de URL, códigos de status, DTOs e mapeamentos. O pacote pode
// traduzir um erro do domínio para HTTP 409, por exemplo, mas não pode decidir
// se um pedido pago pode ou não ser cancelado.
package httpapi
