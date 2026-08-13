package domain

import "errors"

// Este arquivo concentra erros estáveis reconhecidos pelas camadas internas.
// Alguns expressam uma regra de negócio; outros nomeiam situações relevantes
// para os casos de uso, como pedido inexistente ou atualização concorrente.
// Nenhum deles carrega status HTTP, código SQL ou tipo específico do pgx.
var (
	// ErrCustomerRequired informa que o agregado não pode existir sem cliente.
	ErrCustomerRequired = errors.New("cliente é obrigatório")
	// ErrOrderWithoutItems protege a regra de que todo pedido possui ao menos um item.
	ErrOrderWithoutItems = errors.New("pedido deve possuir ao menos um item")
	// ErrInvalidItem reúne as invariantes básicas de produto, preço e quantidade.
	ErrInvalidItem = errors.New("produto, preço e quantidade devem ser válidos")
	// ErrOrderTotalOverflow impede que um cálculo monetário ultrapasse int64.
	ErrOrderTotalOverflow = errors.New("total do pedido excede o limite permitido")
	// ErrOrderNotFound representa uma busca válida cuja identidade não existe.
	ErrOrderNotFound = errors.New("pedido não encontrado")
	// ErrInvalidStatusTransition informa que a mudança viola o ciclo de vida do pedido.
	ErrInvalidStatusTransition = errors.New("somente pedidos pendentes podem mudar de estado")
	// ErrConcurrentModification impede que uma versão antiga sobrescreva uma nova.
	ErrConcurrentModification = errors.New("pedido foi alterado por outra operação")
	// ErrInvalidStoredOrder denuncia dados persistidos incompatíveis com as invariantes.
	ErrInvalidStoredOrder = errors.New("persistência contém um pedido inválido")
)
