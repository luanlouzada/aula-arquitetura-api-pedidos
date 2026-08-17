// Package httpapi traduz o protocolo HTTP para decisões de prontidão,
// admissão e enfileiramento. Probes e métricas não passam pelo limitador de
// negócio, pois precisam continuar observáveis durante sobrecarga e shutdown.
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"aula-08-confiabilidade/internal/admission"
	"aula-08-confiabilidade/internal/health"
	"aula-08-confiabilidade/internal/jobs"
	"aula-08-confiabilidade/internal/telemetry"
)

// Handler é a fronteira entre HTTP e os componentes internos. Ele traduz uma
// decisão de domínio operacional em status e cabeçalhos, mas não processa Job.
type Handler struct {
	instanceID    string
	state         *health.State
	limiter       admission.Gate
	queue         *jobs.Queue
	metrics       *telemetry.Metrics
	workers       int
	ratePerSecond float64
	burst         int
	ids           atomic.Uint64
}

// NewHandler recebe dependências já criadas pelo main. Compartilhar os mesmos
// objetos de estado, fila e métricas garante que probes e /stats observem a
// instância que realmente atende POST /jobs.
func NewHandler(
	instanceID string,
	state *health.State,
	limiter admission.Gate,
	queue *jobs.Queue,
	metrics *telemetry.Metrics,
	workers int,
	ratePerSecond float64,
	burst int,
) *Handler {
	return &Handler{
		instanceID:    instanceID,
		state:         state,
		limiter:       limiter,
		queue:         queue,
		metrics:       metrics,
		workers:       workers,
		ratePerSecond: ratePerSecond,
		burst:         burst,
	}
}

// Routes monta um ServeMux isolado. Probes e estatísticas possuem rotas próprias
// e não passam pelo token bucket usado somente pela operação de negócio.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", h.liveness)
	mux.HandleFunc("GET /readyz", h.readiness)
	mux.HandleFunc("GET /stats", h.stats)
	mux.HandleFunc("POST /jobs", h.enqueue)

	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		// O cliente de carga usa este cabeçalho para mostrar qual processo recebeu
		// cada requisição depois que o NGINX escolheu um upstream.
		response.Header().Set("X-Instance-ID", h.instanceID)
		mux.ServeHTTP(response, request)
	})
}

// enqueueRequest é o contrato recebido do cliente. Campos internos como ID e
// EnqueuedAt são responsabilidade do servidor e não podem ser escolhidos fora.
type enqueueRequest struct {
	OrderID int64 `json:"order_id"`
}

// enqueue executa uma sequência barata para cara: prontidão, validação, limite
// de taxa e fila. Ele retorna 202 assim que a fila aceita; o worker conclui depois.
func (h *Handler) enqueue(response http.ResponseWriter, request *http.Request) {
	h.metrics.RecordRequest()

	// Uma instância em drain continua viva para terminar trabalho, mas recusa
	// novas operações de negócio enquanto sai da rotação.
	if !h.state.Ready() {
		h.metrics.RecordNotReady()
		response.Header().Set("Retry-After", "1")
		writeJSON(response, http.StatusServiceUnavailable, map[string]string{
			"error":  "instância em encerramento",
			"reason": "not_ready",
		})
		return
	}

	request.Body = http.MaxBytesReader(response, request.Body, 1<<20)
	defer request.Body.Close()
	var input enqueueRequest
	if err := decodeJSON(request, &input); err != nil || input.OrderID <= 0 {
		h.metrics.RecordInvalid()
		message := "order_id deve ser positivo"
		if err != nil {
			message = err.Error()
		}
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": message})
		return
	}

	// Allow escolhe rejeitar em vez de esperar. Se não houver uma permissão no
	// token bucket, a requisição nem tenta ocupar a fila.
	// Allow() → esta requisição pode entrar sem ultrapassar o ritmo permitido?
	if !h.limiter.Allow() {
		h.metrics.RecordRateLimited()
		response.Header().Set("Retry-After", "1")
		writeJSON(response, http.StatusTooManyRequests, map[string]string{
			"error":  "limite de entrada excedido",
			"reason": "rate_limit",
		})
		return
	}

	job := jobs.Job{
		ID:         fmt.Sprintf("%s-job-%06d", h.instanceID, h.ids.Add(1)),
		OrderID:    input.OrderID,
		EnqueuedAt: time.Now(),
	}
	if err := h.queue.TryEnqueue(job); err != nil {
		// No caminho normal, MarkNotReady barra novas entradas antes de Close.
		// Esta defesa cobre o handler que já passou por Ready quando o prazo do
		// shutdown exigiu o fechamento forçado do HTTP.
		if errors.Is(err, jobs.ErrQueueClosed) {
			h.metrics.RecordNotReady()
			response.Header().Set("Retry-After", "1")
			writeJSON(response, http.StatusServiceUnavailable, map[string]string{
				"error":  "instância em encerramento",
				"reason": "not_ready",
			})
			return
		}
		if errors.Is(err, jobs.ErrQueueFull) {
			h.metrics.RecordQueueFull()
			response.Header().Set("Retry-After", "1")
			writeJSON(response, http.StatusServiceUnavailable, map[string]string{
				"error":  "fila sem espaço neste instante",
				"reason": "queue_full",
			})
			return
		}
		writeJSON(response, http.StatusInternalServerError, map[string]string{"error": "falha ao enfileirar"})
		return
	}

	h.metrics.RecordAccepted()
	writeJSON(response, http.StatusAccepted, map[string]any{
		"job_id":      job.ID,
		"order_id":    job.OrderID,
		"status":      "queued",
		"queue_depth": h.queue.Depth(),
	})
}

// liveness responde se o processo e o servidor HTTP conseguem executar o
// handler. Ela permanece 200 durante o drain: não estar pronto não é estar morto.
func (h *Handler) liveness(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, map[string]string{
		"status":      "alive",
		"instance_id": h.instanceID,
	})
}

// readiness responde 200 somente quando a instância pode receber trabalho novo.
// Ready() → a instância está aceitando novos trabalhos?
func (h *Handler) readiness(response http.ResponseWriter, _ *http.Request) {
	if !h.state.Ready() {
		writeJSON(response, http.StatusServiceUnavailable, map[string]string{
			"status":      "not_ready",
			"instance_id": h.instanceID,
		})
		return
	}
	writeJSON(response, http.StatusOK, map[string]string{
		"status":      "ready",
		"instance_id": h.instanceID,
	})
}

// stats combina contadores e estado atual. A rota é observacional: consultá-la
// não consome permissão nem muda a capacidade disponível.
func (h *Handler) stats(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, h.metrics.Snapshot(telemetry.Runtime{
		Ready:         h.state.Ready(),
		QueueDepth:    h.queue.Depth(),
		QueueCapacity: h.queue.Capacity(),
		Workers:       h.workers,
		RatePerSecond: h.ratePerSecond,
		Burst:         h.burst,
		Tokens:        h.limiter.Tokens(),
	}))
}

// decodeJSON aceita exatamente um objeto e rejeita campos desconhecidos. Isso
// evita ignorar silenciosamente um nome digitado errado pelo cliente.
func decodeJSON(request *http.Request, target any) error {
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("JSON inválido: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("o corpo deve conter um único objeto JSON")
	}
	return nil
}

// writeJSON centraliza Content-Type e status. Depois de WriteHeader, o status já
// foi enviado e não pode ser trocado por um erro de serialização posterior.
func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
