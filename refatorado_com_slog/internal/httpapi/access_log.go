package httpapi

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// AccessLog envolve o Router e registra exatamente um evento estruturado para
// cada requisição concluída, independentemente do endpoint chamado.
func AccessLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		// O cronômetro começa antes do próximo handler para medir o caminho HTTP
		// completo, inclusive Controller, Service e persistência.
		startedAt := time.Now()
		response := &responseRecorder{ResponseWriter: w, status: http.StatusOK}

		// O Router continua responsável pela resposta. O recorder apenas observa
		// o status e a quantidade de bytes enquanto a resposta é escrita.
		next.ServeHTTP(response, request)

		// O evento é emitido depois da resposta; somente nesse momento conhecemos
		// o status, o total de bytes e a duração completa da requisição.
		logger.InfoContext(
			request.Context(),
			"requisição concluída",
			slog.String("http.method", request.Method),
			slog.String("http.route", routeTemplate(request)),
			slog.Int("http.status_code", response.status),
			slog.Int("http.response_bytes", response.bytes),
			slog.Int64("duration_ms", time.Since(startedAt).Milliseconds()),
		)
	})
}

// routeTemplate separa o método do padrão registrado no ServeMux. Por exemplo,
// "PATCH /orders/{id}/pay" vira "/orders/{id}/pay" porque o método já possui
// seu próprio campo no log. Rotas não encontradas usam um valor estável para
// evitar que IDs e outros caminhos concretos criem campos de alta cardinalidade.
func routeTemplate(request *http.Request) string {
	_, route, found := strings.Cut(request.Pattern, " ")
	if found {
		return route
	}
	if request.Pattern != "" {
		return request.Pattern
	}
	return "unmatched"
}

// responseRecorder envolve o ResponseWriter original para observar a resposta
// sem mudar a forma como o Controller escreve headers e corpo.
type responseRecorder struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

// WriteHeader guarda somente o primeiro status, seguindo a regra de net/http:
// chamadas posteriores não substituem um cabeçalho que já foi enviado.
func (response *responseRecorder) WriteHeader(status int) {
	if response.wroteHeader {
		return
	}
	response.wroteHeader = true
	response.status = status
	response.ResponseWriter.WriteHeader(status)
}

// Write garante o status 200 quando o handler escreve o corpo sem chamar
// WriteHeader e soma os bytes realmente aceitos pelo ResponseWriter original.
func (response *responseRecorder) Write(payload []byte) (int, error) {
	if !response.wroteHeader {
		response.WriteHeader(http.StatusOK)
	}
	written, err := response.ResponseWriter.Write(payload)
	response.bytes += written
	return written, err
}

// Unwrap preserva recursos opcionais do ResponseWriter original.
func (response *responseRecorder) Unwrap() http.ResponseWriter {
	return response.ResponseWriter
}
