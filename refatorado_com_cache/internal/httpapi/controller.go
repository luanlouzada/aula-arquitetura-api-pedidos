package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"

	"aula-pedidos/refatorado_com_cache/internal/application"
	cachelayer "aula-pedidos/refatorado_com_cache/internal/cache"
	"aula-pedidos/refatorado_com_cache/internal/domain"
)

// Este arquivo concentra decisões do protocolo HTTP: rotas, leitura da
// requisição, status da resposta e tradução de erros. Ele não altera campos de
// Order nem executa SQL.

// maxRequestBodyBytes limita o corpo antes da decodificação. Esse é um cuidado
// de transporte para evitar consumo de memória desnecessário; não é uma regra
// de negócio do pedido.
const maxRequestBodyBytes = 64 * 1024

// OrderService é a interface mínima exigida pela camada HTTP. O Controller não
// precisa conhecer a implementação concreta usada em produção. Em Go,
// *application.OrderService satisfaz esta interface implicitamente porque já
// possui esses métodos; não existe uma declaração "implements".
type OrderService interface {
	Create(ctx context.Context, input application.CreateOrderInput) (domain.Order, error)
	Get(ctx context.Context, id int64) (domain.Order, error)
	Pay(ctx context.Context, id int64) (domain.Order, error)
	Cancel(ctx context.Context, id int64) (domain.Order, error)
}

// Controller recebe requisições HTTP e delega trabalho à aplicação. Um
// controller "fino" ainda tem responsabilidades reais: validar o protocolo,
// chamar o caso de uso correto e construir a resposta HTTP.
type Controller struct {
	service         OrderService
	cacheSnapshot   func() cachelayer.Snapshot
	latencySnapshot func() cachelayer.LatencySnapshot
}

// NewRouter monta as rotas e associa cada combinação de método e caminho ao
// handler correspondente. Receber o service pronto mantém a composição das
// dependências fora desta camada.
func NewRouter(
	service OrderService,
	cacheSnapshot func() cachelayer.Snapshot,
	latencySnapshot func() cachelayer.LatencySnapshot,
) http.Handler {
	controller := &Controller{
		service:         service,
		cacheSnapshot:   cacheSnapshot,
		latencySnapshot: latencySnapshot,
	}
	router := http.NewServeMux()
	router.HandleFunc("POST /orders", controller.createOrder)
	router.HandleFunc("GET /orders/{id}", controller.getOrder)
	router.HandleFunc("PATCH /orders/{id}/pay", controller.payOrder)
	router.HandleFunc("PATCH /orders/{id}/cancel", controller.cancelOrder)
	router.HandleFunc("GET /metrics/cache", controller.cacheMetrics)
	return router
}

// cacheMetrics expõe a medição mínima do laboratório. O endpoint mostra
// contadores do processo e o hit ratio; ele não faz parte do domínio de pedidos.
func (controller *Controller) cacheMetrics(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, struct {
		Counters cachelayer.Snapshot        `json:"counters"`
		Latency  cachelayer.LatencySnapshot `json:"latency"`
	}{
		Counters: controller.cacheSnapshot(),
		Latency:  controller.latencySnapshot(),
	})
}

// createOrder trata POST /orders. Ele valida o JSON, converte o DTO para a
// entrada da aplicação e devolve 201 com o DTO de resposta; as validações de
// cliente, itens e total continuam pertencendo ao domínio.
func (controller *Controller) createOrder(w http.ResponseWriter, request *http.Request) {
	var input createOrderRequest
	if err := decodeJSON(w, request, &input); err != nil {
		writeError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	order, err := controller.service.Create(request.Context(), toCreateOrderInput(input))
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toOrderResponse(order))
}

// getOrder trata GET /orders/{id}. A validação do parâmetro é de protocolo; a
// busca propriamente dita é delegada ao caso de uso.
func (controller *Controller) getOrder(w http.ResponseWriter, request *http.Request) {
	id, ok := orderID(w, request)
	if !ok {
		return
	}

	order, err := controller.service.Get(request.Context(), id)
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toOrderResponse(order))
}

// payOrder trata PATCH /orders/{id}/pay. O handler escolhe o caso de uso, mas é
// Order.Pay que decide se a transição de estado é permitida.
func (controller *Controller) payOrder(w http.ResponseWriter, request *http.Request) {
	id, ok := orderID(w, request)
	if !ok {
		return
	}

	order, err := controller.service.Pay(request.Context(), id)
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toOrderResponse(order))
}

// cancelOrder trata PATCH /orders/{id}/cancel. Assim como no pagamento, o HTTP
// não possui autoridade para mudar diretamente o status do pedido.
func (controller *Controller) cancelOrder(w http.ResponseWriter, request *http.Request) {
	id, ok := orderID(w, request)
	if !ok {
		return
	}

	order, err := controller.service.Cancel(request.Context(), id)
	if err != nil {
		writeApplicationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toOrderResponse(order))
}

// orderID converte o segmento {id} da URL para o tipo esperado pela aplicação.
// O bool informa ao handler se pode continuar; quando é false, a resposta 400
// já foi escrita por esta função.
func orderID(w http.ResponseWriter, request *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(request.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "id deve ser um inteiro positivo")
		return 0, false
	}
	return id, true
}

// decodeJSON aplica regras de entrada do protocolo: limita o tamanho, rejeita
// campos desconhecidos e exige exatamente um objeto JSON. Essas verificações
// melhoram o contrato HTTP, mas não substituem as invariantes de NewOrder.
func decodeJSON(w http.ResponseWriter, request *http.Request, destination any) error {
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

// writeApplicationError traduz erros reconhecidos pelas camadas internas para
// o vocabulário HTTP. O domínio continua sem conhecer 404, 409 ou 422; somente
// este adaptador decide qual status representa cada resultado para o cliente.
func writeApplicationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrCustomerRequired),
		errors.Is(err, domain.ErrOrderWithoutItems),
		errors.Is(err, domain.ErrInvalidItem),
		errors.Is(err, domain.ErrOrderTotalOverflow):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, domain.ErrOrderNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrInvalidStatusTransition),
		errors.Is(err, domain.ErrConcurrentModification):
		writeError(w, http.StatusConflict, err.Error())
	default:
		log.Printf("erro interno: %v", err)
		writeError(w, http.StatusInternalServerError, "erro interno")
	}
}

// writeJSON serializa uma resposta e acrescenta os metadados HTTP. O Marshal é
// feito antes de WriteHeader para ainda ser possível responder 500 se o valor
// não puder ser convertido em JSON.
func writeJSON(w http.ResponseWriter, status int, value any) {
	payload, err := json.Marshal(value)
	if err != nil {
		log.Printf("serializar resposta: %v", err)
		http.Error(w, "erro interno", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(append(payload, '\n'))
}

// writeError mantém todas as falhas públicas no mesmo formato {"erro": "..."}.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
