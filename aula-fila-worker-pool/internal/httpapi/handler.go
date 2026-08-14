// Package httpapi traduz HTTP para a operação de enfileirar trabalho. O handler
// confirma aceitação, não conclusão: quem conclui a tarefa é o worker pool.
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"aula-fila-worker-pool/internal/jobs"
	"aula-fila-worker-pool/internal/telemetry"
)

// Handler adapta requisições HTTP para operações da fila e apresenta as
// métricas como JSON. Ele não executa o trabalho assíncrono; sua responsabilidade
// termina quando o Job é aceito ou rejeitado.
//
// ids é atômico porque vários handlers podem gerar identificadores ao mesmo
// tempo. O contador é local ao processo e reinicia junto com a aplicação.
type Handler struct {
	queue   *jobs.Queue
	metrics *telemetry.Metrics
	logger  *slog.Logger
	workers int
	ids     atomic.Uint64
}

// NewHandler recebe a fila e as métricas já criadas no composition root, o ponto
// em que a aplicação é montada. O mesmo objeto Metrics é compartilhado com o
// worker pool, permitindo que /stats reúna aceitação HTTP e processamento em
// segundo plano.
func NewHandler(
	queue *jobs.Queue,
	metrics *telemetry.Metrics,
	workers int,
	logger *slog.Logger,
) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{queue: queue, metrics: metrics, workers: workers, logger: logger}
}

// Routes registra os endpoints do laboratório em um ServeMux próprio e o
// devolve como http.Handler para o servidor. Os padrões incluem método e rota,
// recurso disponível nas versões atuais de net/http.
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /jobs", h.enqueue)
	mux.HandleFunc("GET /stats", h.stats)
	mux.HandleFunc("GET /health", h.health)
	return mux
}

// enqueueRequest descreve somente o contrato de entrada de POST /jobs. Ele não
// é o Job da fila: campos internos como ID e EnqueuedAt são definidos pela API.
type enqueueRequest struct {
	OrderID int64 `json:"order_id"`
}

// enqueue valida uma solicitação e tenta aceitar o trabalho sem bloquear.
// Responde 202 quando o Job entra na fila e 503 com Retry-After quando não há
// capacidade naquele instante. A resposta 202 confirma aceitação, não conclusão.
func (h *Handler) enqueue(response http.ResponseWriter, request *http.Request) {
	// MaxBytesReader limita o corpo a 1 MiB antes da decodificação. Isso impede
	// que uma entrada muito grande consuma memória sem necessidade.
	request.Body = http.MaxBytesReader(response, request.Body, 1<<20)
	defer request.Body.Close()

	var input enqueueRequest
	if err := decodeJSON(request, &input); err != nil {
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if input.OrderID <= 0 {
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": "order_id deve ser positivo"})
		return
	}

	// ID pertence à tentativa de trabalho; OrderID veio do cliente. EnqueuedAt
	// será usado pelo worker para medir o tempo de espera na fila.
	job := jobs.Job{
		ID:         fmt.Sprintf("job-%06d", h.ids.Add(1)),
		OrderID:    input.OrderID,
		EnqueuedAt: time.Now(),
	}
	if err := h.queue.TryEnqueue(job); err != nil {
		if errors.Is(err, jobs.ErrQueueFull) {
			h.metrics.RecordRejected()
			response.Header().Set("Retry-After", "1")
			// A métrica conta todas as rejeições. O evento individual fica em
			// DEBUG para uma sobrecarga não produzir uma segunda sobrecarga de logs.
			h.logger.Debug("trabalho rejeitado: fila cheia",
				slog.String("job_id", job.ID),
				slog.Int64("order_id", job.OrderID),
				slog.Int("queue_depth", h.queue.Depth()),
			)
			writeJSON(response, http.StatusServiceUnavailable, map[string]any{
				"error":       "fila temporariamente cheia",
				"retry_after": 1,
			})
			return
		}
		writeJSON(response, http.StatusInternalServerError, map[string]string{"error": "não foi possível enfileirar"})
		return
	}

	h.metrics.RecordAccepted()
	// Um worker pode retirar o item entre TryEnqueue e Depth. Por isso
	// queue_depth é uma fotografia observacional, não uma confirmação de que o
	// Job ainda está aguardando no buffer.
	writeJSON(response, http.StatusAccepted, map[string]any{
		"job_id":      job.ID,
		"status":      "queued",
		"queue_depth": h.queue.Depth(),
	})
}

// stats combina os contadores com o estado atual da fila e devolve uma
// fotografia operacional. Os números podem mudar enquanto o JSON é enviado.
func (h *Handler) stats(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, h.metrics.Snapshot(
		h.queue.Depth(),
		h.queue.Capacity(),
		h.workers,
	))
}

// health é uma verificação simples de liveness, isto é, confirma que o processo
// HTTP está vivo e responde. Ela não afirma que existe espaço na fila nem testa
// dependências externas, pois este laboratório não possui nenhuma.
func (h *Handler) health(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, map[string]string{"status": "ok"})
}

// decodeJSON exige um único objeto JSON e rejeita campos que não existem no DTO.
// A segunda chamada a Decode deve encontrar io.EOF; qualquer outro resultado
// indica que havia conteúdo adicional depois do primeiro objeto.
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

// writeJSON aplica o cabeçalho, envia o status e serializa value no corpo. O
// cabeçalho e o status precisam ser definidos antes de escrever o JSON, pois a
// primeira escrita confirma a resposta HTTP.
func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
