// Package api concentra propositalmente protocolo HTTP, regras de negócio e
// persistência. O código funciona, mas representa o estado anterior à refatoração.
package api

// Como ler os comentários de acoplamento deste arquivo:
//   - ACOPLAMENTO: uma parte precisa saber detalhes de outra. O sinal prático é
//     mudar o banco e, por causa disso, também precisar mudar o código HTTP;
//   - DUPLICAÇÃO DE CONHECIMENTO: a mesma regra está escrita em vários lugares.
//     Quando a regra muda, todos esses lugares precisam ser encontrados;
//   - BAIXA COESÃO: uma função faz trabalhos que mudam por motivos diferentes;
//   - LIMITE SAUDÁVEL: o detalhe externo existe, mas está no lugar esperado.
//
// Neste arquivo, "handler" é a função chamada pelo Go para receber uma
// requisição HTTP e escrever a resposta, como createOrder e payOrder.

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const maxRequestBodyBytes = 64 * 1024

type API struct {
	// ACOPLAMENTO ENTRE HTTP E POSTGRESQL:
	// API é a parte que recebe requisições HTTP, mas ela guarda diretamente o
	// pool do pgx, a biblioteca usada para conversar com PostgreSQL.
	// Exemplo do efeito: testar a criação de um pedido exige PostgreSQL; não é
	// possível colocar uma implementação simples em memória no lugar.
	database *pgxpool.Pool
}

// UMA STRUCT COM TRÊS TRABALHOS:
//  1. as tags json definem os campos enviados pela API;
//  2. Status e TotalCents guardam o estado usado pelas regras do pedido;
//  3. os campos também recebem valores lidos das colunas do PostgreSQL.
//
// Isso acopla três formatos. Um campo técnico adicionado para o banco pode acabar
// aparecendo no JSON; uma mudança interna de Order obriga revisar a API pública.
type Order struct {
	ID         int64     `json:"id"`
	Customer   string    `json:"cliente"`
	Status     string    `json:"status"`
	TotalCents int64     `json:"total_centavos"`
	Version    int       `json:"versao"`
	CreatedAt  time.Time `json:"criado_em"`
	Items      []Item    `json:"itens"`
}

// A MESMA STRUCT É USADA NA ENTRADA E NA SAÍDA:
// Item recebe os itens enviados pelo cliente e também monta a resposta da API.
// Consequência concreta: o cliente pode enviar subtotal_centavos porque o campo
// existe aqui, embora o servidor ignore esse valor e o recalcule depois.
type Item struct {
	ProductName        string `json:"produto"`
	UnitPriceCents     int64  `json:"preco_unitario_centavos"`
	Quantity           int    `json:"quantidade"`
	SubtotalPriceCents int64  `json:"subtotal_centavos"`
}

type createOrderRequest struct {
	Customer string `json:"cliente"`
	// A reutilização acontece nesta linha. Um tipo criado somente para a entrada
	// poderia listar apenas produto, preço e quantidade, que são dados do cliente.
	Items []Item `json:"itens"`
}

type errorResponse struct {
	Error string `json:"erro"`
}

func New(database *pgxpool.Pool) http.Handler {
	// RECEBER PELO CONSTRUTOR É BOM, MAS NÃO RESOLVE TUDO:
	// o banco não é criado escondido dentro de New; ele chega como parâmetro.
	// Mesmo assim, o parâmetro ainda exige exatamente *pgxpool.Pool. Portanto,
	// a API continua presa ao PostgreSQL e não aceita outra implementação.
	api := &API{database: database}
	router := http.NewServeMux()
	router.HandleFunc("POST /orders", api.createOrder)
	router.HandleFunc("GET /orders/{id}", api.getOrder)
	router.HandleFunc("PATCH /orders/{id}/pay", api.payOrder)
	router.HandleFunc("PATCH /orders/{id}/cancel", api.cancelOrder)
	return router
}

func (api *API) createOrder(w http.ResponseWriter, request *http.Request) {
	// BAIXA COESÃO SIGNIFICA "TRABALHOS DEMAIS NO MESMO LUGAR":
	// este handler lê HTTP, valida regras, calcula o total, abre transação,
	// executa SQL, trata erros e produz JSON. O problema não é apenas ter muitas
	// linhas: cada trabalho muda por um motivo diferente.
	var input createOrderRequest
	// ESTE TRECHO É HTTP: ele entende corpo da requisição e sintaxe JSON.
	if err := decodeJSON(w, request, &input); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	// REGRA DE NEGÓCIO MISTURADA COM RESPOSTA HTTP:
	// "cliente é obrigatório" e "o pedido precisa de item" são regras do pedido.
	// Já 422 e a mensagem JSON são decisões da API HTTP. Como estão no mesmo
	// bloco, reutilizar a regra em uma fila ou testá-la sozinha exige copiar código.
	input.Customer = strings.TrimSpace(input.Customer)
	if input.Customer == "" {
		writeError(w, http.StatusUnprocessableEntity, "cliente é obrigatório")
		return
	}
	if len(input.Items) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "pedido deve possuir ao menos um item")
		return
	}

	// OUTRA REGRA PRESA AO HTTP:
	// validar produto, preço e quantidade e calcular o total são comportamentos do
	// pedido. Aqui só conseguimos executá-los passando por uma requisição HTTP.
	var totalCents int64
	for index := range input.Items {
		item := &input.Items[index]
		item.ProductName = strings.TrimSpace(item.ProductName)
		if item.ProductName == "" || item.UnitPriceCents <= 0 || item.Quantity <= 0 {
			writeError(w, http.StatusUnprocessableEntity, "produto, preço e quantidade devem ser válidos")
			return
		}
		if item.UnitPriceCents > math.MaxInt64/int64(item.Quantity) {
			writeError(w, http.StatusUnprocessableEntity, "total do item excede o limite permitido")
			return
		}
		item.SubtotalPriceCents = item.UnitPriceCents * int64(item.Quantity)
		if totalCents > math.MaxInt64-item.SubtotalPriceCents {
			writeError(w, http.StatusUnprocessableEntity, "total do pedido excede o limite permitido")
			return
		}
		totalCents += item.SubtotalPriceCents
	}

	// HTTP CONTROLANDO O BANCO:
	// esta função, criada para atender uma requisição, também decide onde a
	// transação começa e termina. Uma transação agrupa operações para confirmar
	// todas ou desfazer todas. Se a forma de persistir mudar, o handler muda.
	tx, err := api.database.Begin(request.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	defer func() { _ = tx.Rollback(request.Context()) }()

	// A REGRA DO ESTADO INICIAL NÃO POSSUI UM ÚNICO DONO:
	// PENDENTE aparece aqui, nas regras de pagar/cancelar e no arquivo SQL.
	// Exemplo: ao criar um novo estado inicial, seria necessário procurar todos os
	// textos repetidos e decidir quais precisam mudar.
	order := Order{
		Customer:   input.Customer,
		Status:     "PENDENTE",
		TotalCents: totalCents,
		Version:    1,
		Items:      input.Items,
	}
	// HTTP CONHECENDO A ESTRUTURA DO BANCO:
	// o handler sabe o nome da tabela, o nome das colunas e a ordem dos valores.
	// Exemplo: renomear a coluna customer obriga alterar este código HTTP.
	err = tx.QueryRow(
		request.Context(),
		`INSERT INTO orders (customer, status, total_cents)
		 VALUES ($1, $2, $3)
		 RETURNING id, created_at`,
		order.Customer,
		order.Status,
		order.TotalCents,
	).Scan(&order.ID, &order.CreatedAt)
	if err != nil {
		writeInternalError(w, err)
		return
	}

	for _, item := range order.Items {
		_, err = tx.Exec(
			request.Context(),
			`INSERT INTO order_items (order_id, product_name, unit_price_cents, quantity)
			 VALUES ($1, $2, $3, $4)`,
			order.ID,
			item.ProductName,
			item.UnitPriceCents,
			item.Quantity,
		)
		if err != nil {
			writeInternalError(w, err)
			return
		}
	}

	if err := tx.Commit(request.Context()); err != nil {
		writeInternalError(w, err)
		return
	}
	// BANCO VIRANDO RESPOSTA DIRETAMENTE:
	// a mesma Order preenchida durante a gravação é enviada como JSON. Não existe
	// um ponto separado que escolha quais dados internos podem aparecer na API.
	writeJSON(w, http.StatusCreated, order)
}

func (api *API) getOrder(w http.ResponseWriter, request *http.Request) {
	// DUPLICAÇÃO SIMPLES: converter e validar o ID também acontece em payOrder e
	// cancelOrder. Isso aumenta manutenção, mas é menos grave do que duplicar uma
	// regra de negócio, pois este trecho conhece apenas o formato da URL.
	id, err := strconv.ParseInt(request.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "id deve ser um inteiro positivo")
		return
	}

	var order Order
	// NOVAMENTE, HTTP CONHECE SQL:
	// trocar pgx, alterar a tabela ou renomear uma coluna exige modificar o handler,
	// embora o endereço GET /orders/{id} possa continuar exatamente igual.
	err = api.database.QueryRow(
		request.Context(),
		`SELECT id, customer, status, total_cents, version, created_at
		 FROM orders
		 WHERE id = $1`,
		id,
	).Scan(
		&order.ID,
		&order.Customer,
		&order.Status,
		&order.TotalCents,
		&order.Version,
		&order.CreatedAt,
	)
	// O ERRO DO BANCO ESTÁ LIGADO DIRETAMENTE AO HTTP:
	// pgx.ErrNoRows é a maneira do pgx dizer "a consulta não encontrou linha".
	// 404 é a maneira do HTTP dizer "recurso não encontrado". Este if conecta os
	// dois detalhes; trocar a biblioteca do banco obriga revisar o endpoint HTTP.
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "pedido não encontrado")
		return
	}
	if err != nil {
		writeInternalError(w, err)
		return
	}

	rows, err := api.database.Query(
		request.Context(),
		`SELECT product_name, unit_price_cents, quantity
		 FROM order_items
		 WHERE order_id = $1
		 ORDER BY id`,
		order.ID,
	)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	defer rows.Close()

	order.Items = make([]Item, 0)
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ProductName, &item.UnitPriceCents, &item.Quantity); err != nil {
			writeInternalError(w, err)
			return
		}
		item.SubtotalPriceCents = item.UnitPriceCents * int64(item.Quantity)
		order.Items = append(order.Items, item)
	}
	if err := rows.Err(); err != nil {
		writeInternalError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, order)
}

func (api *API) payOrder(w http.ResponseWriter, request *http.Request) {
	// MUITAS ETAPAS PRESAS EM UMA ORDEM FIXA:
	// HTTP → transação → bloqueio → regra → UPDATE → itens → commit → JSON.
	// Algumas etapas realmente precisam de ordem, mas aqui elas formam a única
	// maneira de "pagar". Para testar somente "pedido pendente pode ser pago",
	// somos obrigados a atravessar HTTP e PostgreSQL também.
	id, err := strconv.ParseInt(request.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "id deve ser um inteiro positivo")
		return
	}

	// A transação é necessária. O ponto ruim é o código HTTP precisar saber
	// como abri-la, desfazê-la e confirmá-la.
	tx, err := api.database.Begin(request.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	defer func() { _ = tx.Rollback(request.Context()) }()

	// DETALHE DE CONCORRÊNCIA DO POSTGRESQL DENTRO DO HANDLER:
	// FOR UPDATE bloqueia esta linha para impedir duas mudanças simultâneas.
	// O bloqueio é útil; o acoplamento vem de a camada HTTP conhecer como o
	// PostgreSQL realiza essa proteção.
	var order Order
	err = tx.QueryRow(
		request.Context(),
		`SELECT id, customer, status, total_cents, version, created_at
		 FROM orders
		 WHERE id = $1
		 FOR UPDATE`,
		id,
	).Scan(
		&order.ID,
		&order.Customer,
		&order.Status,
		&order.TotalCents,
		&order.Version,
		&order.CreatedAt,
	)
	// O mesmo acoplamento de erro aparece novamente: pgx.ErrNoRows decide o 404.
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "pedido não encontrado")
		return
	}
	if err != nil {
		writeInternalError(w, err)
		return
	}
	// REGRA DE NEGÓCIO + DECISÃO HTTP NO MESMO IF:
	// "somente pedido pendente muda de estado" é regra do negócio; 409 é HTTP.
	// A regra também está copiada em cancelOrder. Se ela mudar, os dois blocos
	// precisam ser alterados para manter as respostas coerentes.
	if order.Status != "PENDENTE" {
		writeError(w, http.StatusConflict, "somente pedidos pendentes podem mudar de estado")
		return
	}

	// QUEM DECIDE O ESTADO É O HANDLER:
	// esta atribuição faz o código HTTP ser o dono da mudança para PAGO.
	// Apenas mover Order para outro arquivo não resolveria: a decisão continuaria
	// aqui, fora de um ponto único que proteja as regras do pedido.
	order.Status = "PAGO"
	err = tx.QueryRow(
		request.Context(),
		`UPDATE orders
		 SET status = $2, version = version + 1
		 WHERE id = $1
		 RETURNING version`,
		order.ID,
		order.Status,
	).Scan(&order.Version)
	if err != nil {
		writeInternalError(w, err)
		return
	}

	// O FORMATO DA RESPOSTA FAZ O BLOQUEIO DURAR MAIS:
	// o pagamento já foi decidido e o UPDATE foi executado, mas a transação ainda
	// não foi confirmada. Os itens só são buscados porque a API quer devolvê-los no
	// JSON. Até o commit, a linha continua bloqueada durante esta consulta. Assim,
	// uma escolha de resposta HTTP aumenta o tempo de bloqueio no banco.
	rows, err := tx.Query(
		request.Context(),
		`SELECT product_name, unit_price_cents, quantity
		 FROM order_items
		 WHERE order_id = $1
		 ORDER BY id`,
		order.ID,
	)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	order.Items = make([]Item, 0)
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ProductName, &item.UnitPriceCents, &item.Quantity); err != nil {
			rows.Close()
			writeInternalError(w, err)
			return
		}
		item.SubtotalPriceCents = item.UnitPriceCents * int64(item.Quantity)
		order.Items = append(order.Items, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		writeInternalError(w, err)
		return
	}

	if err := tx.Commit(request.Context()); err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (api *API) cancelOrder(w http.ResponseWriter, request *http.Request) {
	// ESTE FLUXO É QUASE UMA CÓPIA DE payOrder:
	// buscar com bloqueio, tratar ausência, verificar PENDENTE, atualizar, buscar
	// itens e responder aparecem nos dois lugares. Se a maneira de bloquear ou
	// carregar um pedido mudar, será preciso lembrar de modificar os dois fluxos.
	id, err := strconv.ParseInt(request.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "id deve ser um inteiro positivo")
		return
	}

	tx, err := api.database.Begin(request.Context())
	if err != nil {
		writeInternalError(w, err)
		return
	}
	defer func() { _ = tx.Rollback(request.Context()) }()

	var order Order
	err = tx.QueryRow(
		request.Context(),
		`SELECT id, customer, status, total_cents, version, created_at
		 FROM orders
		 WHERE id = $1
		 FOR UPDATE`,
		id,
	).Scan(
		&order.ID,
		&order.Customer,
		&order.Status,
		&order.TotalCents,
		&order.Version,
		&order.CreatedAt,
	)
	// REPETIÇÃO: o erro específico do pgx volta a decidir uma resposta HTTP 404.
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "pedido não encontrado")
		return
	}
	if err != nil {
		writeInternalError(w, err)
		return
	}
	// DUPLICAÇÃO DA REGRA:
	// este é o mesmo teste existente em payOrder. Se futuramente um pedido em outro
	// estado puder ser cancelado, este ponto e o outro poderão divergir por esquecimento.
	if order.Status != "PENDENTE" {
		writeError(w, http.StatusConflict, "somente pedidos pendentes podem mudar de estado")
		return
	}

	// Novamente, o próprio handler altera o estado. A regra de cancelamento não está
	// protegida por um objeto ou função de negócio que possa ser usada sem HTTP.
	order.Status = "CANCELADO"
	err = tx.QueryRow(
		request.Context(),
		`UPDATE orders
		 SET status = $2, version = version + 1
		 WHERE id = $1
		 RETURNING version`,
		order.ID,
		order.Status,
	).Scan(&order.Version)
	if err != nil {
		writeInternalError(w, err)
		return
	}

	// O mesmo efeito de payOrder se repete: os itens são necessários para o JSON,
	// não para cancelar. Mesmo assim, são lidos antes do commit e prolongam o lock.
	rows, err := tx.Query(
		request.Context(),
		`SELECT product_name, unit_price_cents, quantity
		 FROM order_items
		 WHERE order_id = $1
		 ORDER BY id`,
		order.ID,
	)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	order.Items = make([]Item, 0)
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ProductName, &item.UnitPriceCents, &item.Quantity); err != nil {
			rows.Close()
			writeInternalError(w, err)
			return
		}
		item.SubtotalPriceCents = item.UnitPriceCents * int64(item.Quantity)
		order.Items = append(order.Items, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		writeInternalError(w, err)
		return
	}

	if err := tx.Commit(request.Context()); err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func decodeJSON(w http.ResponseWriter, request *http.Request, destination any) error {
	// LIMITE SAUDÁVEL: esta função só entende HTTP e JSON, exatamente o trabalho
	// esperado dela. Ela se tornaria um problema se uma regra de pedido precisasse
	// chamá-la para funcionar.
	body := http.MaxBytesReader(w, request.Body, maxRequestBodyBytes)
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("corpo deve conter somente um objeto JSON")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	payload, err := json.Marshal(value)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(append(payload, '\n'))
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func writeInternalError(w http.ResponseWriter, err error) {
	// LIMITE SAUDÁVEL: registrar o erro real e enviar apenas "erro interno" ao
	// cliente é uma responsabilidade HTTP adequada. O problema visto acima é o
	// handler conhecer erros específicos do pgx e misturá-los com regras do pedido.
	log.Printf("erro interno: %v", err)
	writeError(w, http.StatusInternalServerError, "erro interno")
}
