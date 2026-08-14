// Package httpapi traduz HTTP para a operação de enfileirar uma comanda.
// O handler confirma aceitação, não conclusão: quem prepara o prato é o pool.
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"exercicio/internal/kitchen"
)

// Handler adapta requisições HTTP para operações da fila. Ele não cozinha;
// sua responsabilidade termina quando o Ticket é aceito ou rejeitado.
type Handler struct {
	queue *kitchen.Queue
	ids   atomic.Uint64
}

func NewHandler(queue *kitchen.Queue) *Handler {
	return &Handler{queue: queue}
}

func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /tickets", h.enqueue)
	mux.HandleFunc("GET /health", h.health)
	return mux
}

type enqueueRequest struct {
	Dish string `json:"prato"`
}

func (h *Handler) enqueue(response http.ResponseWriter, request *http.Request) {
	request.Body = http.MaxBytesReader(response, request.Body, 1<<20)
	defer request.Body.Close()

	var input enqueueRequest
	if err := decodeJSON(request, &input); err != nil {
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if input.Dish == "" {
		writeJSON(response, http.StatusBadRequest, map[string]string{"error": "prato é obrigatório"})
		return
	}

	ticket := kitchen.Ticket{
		ID:         fmt.Sprintf("ticket-%06d", h.ids.Add(1)),
		Dish:       input.Dish,
		EnqueuedAt: time.Now(),
	}
	if err := h.queue.TryEnqueue(ticket); err != nil {
		if errors.Is(err, kitchen.ErrQueueFull) {
			response.Header().Set("Retry-After", "1")
			writeJSON(response, http.StatusServiceUnavailable, map[string]any{
				"error":       "cozinha lotada",
				"retry_after": 1,
			})
			return
		}
		writeJSON(response, http.StatusInternalServerError, map[string]string{"error": "não foi possível enfileirar"})
		return
	}

	writeJSON(response, http.StatusAccepted, map[string]any{
		"ticket_id":   ticket.ID,
		"status":      "queued",
		"prato":       ticket.Dish,
		"queue_depth": h.queue.Depth(),
	})
}

func (h *Handler) health(response http.ResponseWriter, _ *http.Request) {
	writeJSON(response, http.StatusOK, map[string]string{"status": "ok"})
}

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

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
