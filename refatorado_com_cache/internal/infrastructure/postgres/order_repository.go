package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"aula-pedidos/refatorado_com_cache/internal/application"
	"aula-pedidos/refatorado_com_cache/internal/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Este arquivo traduz entre o agregado de domínio e as tabelas orders e
// order_items. Essa tradução é detalhe de infraestrutura: a aplicação continua
// enxergando um único OrderRepository.

// OrderRepository é a implementação PostgreSQL do contrato da aplicação. Ela
// conhece SQL, pgx, transações e a forma de reconstituir o agregado, mas não
// decide transições de estado nem altera campos privados de Order diretamente.
type OrderRepository struct {
	database *pgxpool.Pool
}

// NewOrderRepository recebe o pool já configurado pelo composition root. O
// construtor não abre conexões nem lê ambiente, o que mantém cada etapa de
// inicialização explícita em cmd/api.
func NewOrderRepository(database *pgxpool.Pool) *OrderRepository {
	return &OrderRepository{database: database}
}

// Create grava a raiz e seus itens na mesma transação. Embora existam duas
// tabelas, elas representam um único agregado; ou tudo é confirmado, ou nada é.
// O retorno inclui ID, versão e data gerados pelo PostgreSQL.
func (repository *OrderRepository) Create(ctx context.Context, order domain.Order) (domain.Order, error) {
	tx, err := repository.database.Begin(ctx)
	if err != nil {
		return domain.Order{}, fmt.Errorf("iniciar criação do pedido: %w", err)
	}
	// Rollback funciona como rede de segurança para qualquer retorno antecipado.
	// Depois de Commit ele não desfaz nada e seu possível erro pode ser ignorado.
	defer func() { _ = tx.Rollback(ctx) }()

	// Primeiro persistimos a raiz para obter a chave usada pelos order_items.
	var id int64
	var version int
	var createdAt time.Time
	err = tx.QueryRow(
		ctx,
		`INSERT INTO orders (customer, status, total_cents)
		 VALUES ($1, $2, $3)
		 RETURNING id, version, created_at`,
		order.Customer(),
		order.Status(),
		order.TotalCents(),
	).Scan(&id, &version, &createdAt)
	if err != nil {
		return domain.Order{}, fmt.Errorf("inserir pedido: %w", err)
	}

	// Os itens não possuem Repository público próprio: são persistidos como
	// parte interna do mesmo agregado e compartilham a transação da raiz.
	for _, item := range order.Items() {
		_, err = tx.Exec(
			ctx,
			`INSERT INTO order_items (order_id, product_name, unit_price_cents, quantity)
			 VALUES ($1, $2, $3, $4)`,
			id,
			item.ProductName(),
			item.UnitPriceCents(),
			item.Quantity(),
		)
		if err != nil {
			return domain.Order{}, fmt.Errorf("inserir item do pedido: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.Order{}, fmt.Errorf("confirmar criação do pedido: %w", err)
	}

	// RestoreOrder devolve um objeto completo sem permitir que a infraestrutura
	// monte livremente campos privados ou introduza um estado inválido.
	persisted, err := domain.RestoreOrder(
		id,
		order.Customer(),
		order.Status(),
		order.TotalCents(),
		version,
		createdAt,
		order.Items(),
	)
	if err != nil {
		return domain.Order{}, fmt.Errorf("reconstituir pedido criado: %w", err)
	}
	return persisted, nil
}

// FindByID lê a raiz, carrega seus itens e reconstitui o agregado completo. O
// caso de uso não recebe linhas de tabela nem precisa conhecer essa composição.
func (repository *OrderRepository) FindByID(ctx context.Context, id int64) (domain.Order, error) {
	var customer string
	var status domain.Status
	var totalCents int64
	var version int
	var createdAt time.Time

	err := repository.database.QueryRow(
		ctx,
		`SELECT customer, status, total_cents, version, created_at
		 FROM orders
		 WHERE id = $1`,
		id,
	).Scan(&customer, &status, &totalCents, &version, &createdAt)
	// pgx.ErrNoRows é um detalhe do driver. Traduzimos para um erro estável que
	// a aplicação e a camada HTTP conseguem reconhecer com errors.Is.
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Order{}, domain.ErrOrderNotFound
	}
	if err != nil {
		return domain.Order{}, fmt.Errorf("buscar pedido: %w", err)
	}

	items, err := repository.findItems(ctx, id)
	if err != nil {
		return domain.Order{}, err
	}

	// Mesmo vindo do banco, o estado passa novamente pelas invariantes. Banco
	// persistente não é motivo para confiar cegamente nos dados carregados.
	order, err := domain.RestoreOrder(id, customer, status, totalCents, version, createdAt, items)
	if err != nil {
		return domain.Order{}, fmt.Errorf("reconstituir pedido %d: %w", id, err)
	}
	return order, nil
}

// Save persiste a mudança de estado usando concorrência otimista. A cláusula
// WHERE exige a mesma versão que foi carregada; se outra operação já a mudou,
// esta atualização não sobrescreve silenciosamente o dado mais novo.
func (repository *OrderRepository) Save(ctx context.Context, order domain.Order) (domain.Order, error) {
	var nextVersion int
	err := repository.database.QueryRow(
		ctx,
		`UPDATE orders
		 SET status = $2, version = version + 1
		 WHERE id = $1 AND version = $3
		 RETURNING version`,
		order.ID(),
		order.Status(),
		order.Version(),
	).Scan(&nextVersion)
	// Como o pedido foi carregado antes pelo caso de uso, nenhuma linha atualizada
	// significa que sua versão ficou desatualizada (ou que ele deixou de existir).
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Order{}, domain.ErrConcurrentModification
	}
	if err != nil {
		return domain.Order{}, fmt.Errorf("atualizar pedido: %w", err)
	}

	// O banco incrementou a versão; reconstituímos o retorno para que o chamador
	// não continue trabalhando com a versão antiga mantida em memória.
	persisted, err := domain.RestoreOrder(
		order.ID(),
		order.Customer(),
		order.Status(),
		order.TotalCents(),
		nextVersion,
		order.CreatedAt(),
		order.Items(),
	)
	if err != nil {
		return domain.Order{}, fmt.Errorf("reconstituir pedido atualizado: %w", err)
	}
	return persisted, nil
}

// findItems lê os registros filhos usados por FindByID. É uma função privada
// porque item não é carregado como agregado independente nesta API.
func (repository *OrderRepository) findItems(ctx context.Context, orderID int64) ([]domain.Item, error) {
	rows, err := repository.database.Query(
		ctx,
		`SELECT product_name, unit_price_cents, quantity
		 FROM order_items
		 WHERE order_id = $1
		 ORDER BY id`,
		orderID,
	)
	if err != nil {
		return nil, fmt.Errorf("buscar itens do pedido: %w", err)
	}
	defer rows.Close()

	items := make([]domain.Item, 0)
	for rows.Next() {
		var productName string
		var unitPriceCents int64
		var quantity int
		if err := rows.Scan(&productName, &unitPriceCents, &quantity); err != nil {
			return nil, fmt.Errorf("ler item do pedido: %w", err)
		}
		// NewItem impede que uma linha inválida escape do adaptador e passe a ser
		// tratada pela aplicação como um objeto de domínio legítimo.
		item, err := domain.NewItem(productName, unitPriceCents, quantity)
		if err != nil {
			return nil, fmt.Errorf("reconstituir item do pedido: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("percorrer itens do pedido: %w", err)
	}
	return items, nil
}

// Esta atribuição não executa em runtime. Ela pede ao compilador que confirme
// que *postgres.OrderRepository satisfaz o contrato application.OrderRepository.
var _ application.OrderRepository = (*OrderRepository)(nil)
