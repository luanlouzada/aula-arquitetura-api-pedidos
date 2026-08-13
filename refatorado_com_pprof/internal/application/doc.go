// Package application contém os casos de uso oferecidos pelo sistema.
//
// A camada coordena o fluxo de uma operação: recebe dados já separados do
// protocolo HTTP, cria ou carrega o agregado, pede ao domínio que execute a
// regra e solicita persistência por meio de um contrato. Ela decide a ordem
// dessas etapas, mas não decide as invariantes do pedido nem executa SQL.
package application
