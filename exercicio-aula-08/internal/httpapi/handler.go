// Package httpapi traduz as políticas internas para contratos HTTP. Ele não
// gera relatórios; sua responsabilidade termina na aceitação ou rejeição.
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"exercicio-aula-08/internal/admission"
	"exercicio-aula-08/internal/exports"
	"exercicio-aula-08/internal/health"
	"exercicio-aula-08/internal/telemetry"
)

// Handler compartilha o mesmo estado, limitador, fila e métricas usados pela
// instância. Assim, decisões e endpoints operacionais observam dados reais.
type Handler struct {
	instanceID    string
	state         *health.State
	limiter       admission.Gate
	queue         *exports.Queue
	metrics       *telemetry.Metrics
	workers       int
	ratePerSecond float64
	burst         int
	ids           atomic.Uint64
}

// NewHandler recebe dependências prontas do composition root. O construtor não
// lê ambiente nem cria goroutines, deixando o ciclo de vida sob controle do main.
func NewHandler(
	instanceID string,
	state *health.State,
	limiter admission.Gate,
	queue *exports.Queue,
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

// Routes registra negócio, probes e métricas. Somente POST /exports passa pelas
// políticas de admissão; limitar /livez esconderia a saúde durante sobrecarga.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", h.liveness)
	mux.HandleFunc("GET /readyz", h.readiness)
	mux.HandleFunc("GET /stats", h.stats)
	mux.HandleFunc("POST /exports", h.enqueue)
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("X-Instance-ID", h.instanceID)
		mux.ServeHTTP(response, request)
	})
}

// enqueueRequest contém apenas o dado controlado pelo cliente. ID e horário de
// entrada são criados pelo servidor para preservar consistência interna.
type enqueueRequest struct {
	Report string `json:"report"`
}

// enqueue avalia prontidão, validação, taxa e fila nessa ordem. Um 202 encerra a
// requisição HTTP, mas a exportação continua em uma goroutine do Pool.
func (h *Handler) enqueue(response http.ResponseWriter, request *http.Request) {
	h.metrics.RecordRequest()
	if h.rejectWhenNotReady(response) {
		return
	}

	request.Body = http.MaxBytesReader(response, request.Body, 1<<20)
	defer request.Body.Close()
	var input enqueueRequest
	if err := decodeJSON(request, &input); err != nil || input.Report == "" {
		h.metrics.RecordInvalid()
		message := "report é obrigatório"
		if err != nil {
			message = err.Error()
		}
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": message})
		return
	}

	if h.rejectWhenRateLimited(response) {
		return
	}

	item := exports.Export{
		ID:         fmt.Sprintf("%s-export-%06d", h.instanceID, h.ids.Add(1)),
		Report:     input.Report,
		EnqueuedAt: time.Now(),
	}
	if err := h.queue.TryEnqueue(item); err != nil {
		// A Queue fornecida pode ser fechada enquanto um handler antigo termina.
		// Esse estado pertence ao shutdown, não a uma falha interna da aplicação.
		if errors.Is(err, exports.ErrQueueClosed) {
			h.metrics.RecordNotReady()
			response.Header().Set("Retry-After", "1")
			writeJSON(response, http.StatusServiceUnavailable, map[string]string{
				"error":  "instância em encerramento",
				"reason": "not_ready",
			})
			return
		}
		if errors.Is(err, exports.ErrQueueFull) {
			h.metrics.RecordQueueFull()
			response.Header().Set("Retry-After", "1")
			writeJSON(response, http.StatusServiceUnavailable, map[string]string{
				"error":  "fila de exportações sem espaço",
				"reason": "queue_full",
			})
			return
		}
		writeJSON(response, http.StatusInternalServerError, map[string]string{"error": "falha ao enfileirar"})
		return
	}

	h.metrics.RecordAccepted()
	writeJSON(response, http.StatusAccepted, map[string]any{
		"export_id":   item.ID,
		"report":      item.Report,
		"status":      "queued",
		"queue_depth": h.queue.Depth(),
	})
}

// rejectWhenNotReady aplica ao HTTP a decisão operacional de prontidão. O retorno
// informa a enqueue se uma resposta já foi enviada e o fluxo deve parar.
func (h *Handler) rejectWhenNotReady(response http.ResponseWriter) bool {
	// TODO 6: implemente o ramo not-ready definido pelo contrato e pelos testes.
	return false
}

// rejectWhenRateLimited aplica ao HTTP a decisão do controle de admissão. O
// retorno informa a enqueue se uma resposta já foi enviada e o fluxo deve parar.
func (h *Handler) rejectWhenRateLimited(response http.ResponseWriter) bool {
	// TODO 7: implemente o ramo de limitação definido pelo contrato e pelos testes.
	return false
}

// liveness responde se o processo ainda executa HTTP. Ela continua 200 durante
// o drain porque o processo precisa permanecer vivo para concluir a fila.
func (h *Handler) liveness(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, map[string]string{
		"status":      "alive",
		"instance_id": h.instanceID,
	})
}

// readiness responde se a instância deve receber exportações novas agora. Não
// pronta produz 503 sem afirmar que o processo morreu.
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

// stats expõe uma fotografia local e não consome token. Chamar o NGINX pode cair
// em qualquer instância; portas 8091–8093 permitem comparar cada uma diretamente.
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

// decodeJSON exige um único objeto e rejeita campos desconhecidos, evitando que
// erros de digitação sejam aceitos silenciosamente.
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

// writeJSON define cabeçalho e status antes de serializar. Depois de WriteHeader,
// o status já foi enviado ao cliente e não pode ser alterado.
func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
