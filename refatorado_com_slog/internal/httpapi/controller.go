package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"aula-pedidos/refatorado_com_slog/internal/application"
	"aula-pedidos/refatorado_com_slog/internal/domain"
)

// Este arquivo mantém as mesmas responsabilidades HTTP do projeto refatorado.
// A diferença é que falhas técnicas são registradas pelo logger configurado no
// main, sem colocar configuração ou formatação de logs no Controller.

const maxRequestBodyBytes = 64 * 1024

// OrderService é a interface mínima consumida pelo Controller. Tanto
// application.OrderService quanto logging.LoggedOrderService a satisfazem
// implicitamente.
type OrderService interface {
	Create(ctx context.Context, input application.CreateOrderInput) (domain.Order, error)
	Get(ctx context.Context, id int64) (domain.Order, error)
	Pay(ctx context.Context, id int64) (domain.Order, error)
	Cancel(ctx context.Context, id int64) (domain.Order, error)
}

type Controller struct {
	service OrderService
	logger  *slog.Logger
}

// NewRouter recebe o caso de uso e o logger já construídos. Ele não lê
// variáveis de ambiente e não decide o formato ou o nível dos registros.
func NewRouter(service OrderService, logger *slog.Logger) http.Handler {
	controller := &Controller{service: service, logger: logger}
	router := http.NewServeMux()
	router.HandleFunc("POST /orders", controller.createOrder)
	router.HandleFunc("GET /orders/{id}", controller.getOrder)
	router.HandleFunc("PATCH /orders/{id}/pay", controller.payOrder)
	router.HandleFunc("PATCH /orders/{id}/cancel", controller.cancelOrder)
	return router
}

// createOrder valida a representação HTTP, converte o DTO para a entrada da
// aplicação e devolve o agregado criado como DTO de resposta.
func (controller *Controller) createOrder(w http.ResponseWriter, request *http.Request) {
	var input createOrderRequest
	if err := decodeJSON(w, request, &input); err != nil {
		controller.writeError(w, http.StatusBadRequest, "JSON inválido")
		return
	}

	order, err := controller.service.Create(request.Context(), toCreateOrderInput(input))
	if err != nil {
		controller.writeApplicationError(request.Context(), w, err)
		return
	}
	controller.writeJSON(w, http.StatusCreated, toOrderResponse(order))
}

// getOrder lê o ID da rota, chama o caso de uso e traduz o resultado para JSON.
func (controller *Controller) getOrder(w http.ResponseWriter, request *http.Request) {
	id, ok := controller.orderID(w, request)
	if !ok {
		return
	}

	order, err := controller.service.Get(request.Context(), id)
	if err != nil {
		controller.writeApplicationError(request.Context(), w, err)
		return
	}
	controller.writeJSON(w, http.StatusOK, toOrderResponse(order))
}

// payOrder cuida apenas do protocolo: o domínio, chamado pelo Service, é quem
// decide se a transição para pago é permitida.
func (controller *Controller) payOrder(w http.ResponseWriter, request *http.Request) {
	id, ok := controller.orderID(w, request)
	if !ok {
		return
	}

	order, err := controller.service.Pay(request.Context(), id)
	if err != nil {
		controller.writeApplicationError(request.Context(), w, err)
		return
	}
	controller.writeJSON(w, http.StatusOK, toOrderResponse(order))
}

// cancelOrder segue o mesmo fluxo de payOrder, mas pede ao caso de uso a
// transição para cancelado.
func (controller *Controller) cancelOrder(w http.ResponseWriter, request *http.Request) {
	id, ok := controller.orderID(w, request)
	if !ok {
		return
	}

	order, err := controller.service.Cancel(request.Context(), id)
	if err != nil {
		controller.writeApplicationError(request.Context(), w, err)
		return
	}
	controller.writeJSON(w, http.StatusOK, toOrderResponse(order))
}

// orderID converte o parâmetro textual da rota em uma identidade válida para a
// aplicação. O bool informa ao handler se a resposta de erro já foi escrita.
func (controller *Controller) orderID(w http.ResponseWriter, request *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(request.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		controller.writeError(w, http.StatusBadRequest, "id deve ser um inteiro positivo")
		return 0, false
	}
	return id, true
}

// decodeJSON limita o tamanho do corpo, rejeita campos desconhecidos e garante
// que exista exatamente um objeto JSON na requisição.
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

// writeApplicationError traduz recusas previstas e registra somente uma falha
// técnica inesperada. Erros de negócio conhecidos viram 4xx e não são tratados
// como pane da aplicação.
func (controller *Controller) writeApplicationError(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrCustomerRequired),
		errors.Is(err, domain.ErrOrderWithoutItems),
		errors.Is(err, domain.ErrInvalidItem),
		errors.Is(err, domain.ErrOrderTotalOverflow):
		controller.writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, domain.ErrOrderNotFound):
		controller.writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, domain.ErrInvalidStatusTransition),
		errors.Is(err, domain.ErrConcurrentModification):
		controller.writeError(w, http.StatusConflict, err.Error())
	default:
		controller.logger.ErrorContext(ctx, "requisição falhou", slog.Any("error", err))
		controller.writeError(w, http.StatusInternalServerError, "erro interno")
	}
}

// writeJSON serializa a resposta, define Content-Type e só então envia status e
// corpo. Uma falha de serialização é registrada como erro técnico.
func (controller *Controller) writeJSON(w http.ResponseWriter, status int, value any) {
	payload, err := json.Marshal(value)
	if err != nil {
		controller.logger.Error("serializar resposta HTTP", slog.Any("error", err))
		http.Error(w, "erro interno", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(append(payload, '\n'))
}

// writeError mantém todas as respostas de erro no mesmo formato JSON.
func (controller *Controller) writeError(w http.ResponseWriter, status int, message string) {
	controller.writeJSON(w, status, errorResponse{Error: message})
}
