package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// Este arquivo contém um candidato de otimização para a Aula 05. Ele termina
// em _test.go de propósito: primeiro comparamos comportamento e custo; só
// depois uma otimização comprovadamente útil deve entrar no código da API.

const maxPooledJSONBufferCapacity = 64 * 1024

var responseBufferPool = sync.Pool{
	New: func() any {
		// O Pool guarda ponteiros porque bytes.Buffer não deve ser copiado depois
		// de usado e porque guardar o valor na interface any poderia alocá-lo.
		return new(bytes.Buffer)
	},
}

// O sink mantém o tamanho da última resposta observável fora do benchmark e
// impede que uma otimização futura do compilador considere a escrita inútil.
var benchmarkResponseSizeSink int

// writeJSONWithNewBuffer usa o mesmo Encoder da versão com Pool, porém cria
// um bytes.Buffer em toda chamada. Essa terceira versão separa duas mudanças:
// trocar a estratégia de codificação e reutilizar o buffer temporário.
func writeJSONWithNewBuffer(w http.ResponseWriter, status int, value any) {
	buffer := new(bytes.Buffer)
	if err := json.NewEncoder(buffer).Encode(value); err != nil {
		log.Printf("serializar resposta: %v", err)
		http.Error(w, "erro interno", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buffer.Bytes())
}

// writeJSONWithPool é uma versão candidata de writeJSON. O buffer temporário
// pertence somente a esta chamada: ele é obtido, zerado, usado e devolvido.
// O conteúdo é escrito antes da devolução, portanto nenhuma resposta conserva
// uma referência aos bytes que serão reutilizados por outra requisição.
func writeJSONWithPool(w http.ResponseWriter, status int, value any) {
	buffer := responseBufferPool.Get().(*bytes.Buffer)
	buffer.Reset()

	defer func() {
		// Um pedido excepcionalmente grande pode fazer o buffer crescer muito.
		// Nesse caso ele não volta ao Pool, para que uma resposta rara não retenha
		// uma grande área de memória indefinidamente.
		if buffer.Cap() <= maxPooledJSONBufferCapacity {
			responseBufferPool.Put(buffer)
		}
	}()

	// Encoder.Encode acrescenta a mesma quebra de linha que writeJSON adiciona
	// depois de json.Marshal. A serialização continua antes de WriteHeader, o
	// que permite devolver 500 se o valor não puder ser convertido.
	if err := json.NewEncoder(buffer).Encode(value); err != nil {
		log.Printf("serializar resposta: %v", err)
		http.Error(w, "erro interno", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(buffer.Bytes())
}

// TestWriteJSONCandidatesPreserveTheResponse prova que os candidatos mantêm o
// contrato observável antes de compararmos desempenho.
func TestWriteJSONCandidatesPreserveTheResponse(t *testing.T) {
	response := toOrderResponse(newOrderForMapper(t, 3))

	want := httptest.NewRecorder()
	writeJSON(want, http.StatusCreated, response)

	strategies := map[string]func(http.ResponseWriter, int, any){
		"novo bytes.Buffer": writeJSONWithNewBuffer,
		"sync.Pool":         writeJSONWithPool,
	}
	for name, strategy := range strategies {
		t.Run(name, func(t *testing.T) {
			got := httptest.NewRecorder()
			strategy(got, http.StatusCreated, response)

			if got.Code != want.Code {
				t.Fatalf("status = %d, implementação atual = %d", got.Code, want.Code)
			}
			if got.Header().Get("Content-Type") != want.Header().Get("Content-Type") {
				t.Fatalf(
					"Content-Type = %q, implementação atual = %q",
					got.Header().Get("Content-Type"),
					want.Header().Get("Content-Type"),
				)
			}
			if got.Body.String() != want.Body.String() {
				t.Fatalf("corpo difere da implementação atual")
			}
		})
	}
}

// TestWriteJSONCandidatesPreserveSerializationError protege também o caminho
// de falha: nenhum candidato pode enviar 200 ou um JSON parcial quando recebe
// um valor que encoding/json não sabe representar.
func TestWriteJSONCandidatesPreserveSerializationError(t *testing.T) {
	// As três estratégias registram o erro esperado. Silenciamos somente este
	// teste para que a saída do benchmark mostre os resultados, não quinze linhas
	// repetidas quando make pool-bench usa -count=5.
	previousLogOutput := log.Writer()
	log.SetOutput(io.Discard)
	t.Cleanup(func() {
		log.SetOutput(previousLogOutput)
	})

	value := make(chan int)

	want := httptest.NewRecorder()
	writeJSON(want, http.StatusOK, value)

	strategies := map[string]func(http.ResponseWriter, int, any){
		"novo bytes.Buffer": writeJSONWithNewBuffer,
		"sync.Pool":         writeJSONWithPool,
	}
	for name, strategy := range strategies {
		t.Run(name, func(t *testing.T) {
			got := httptest.NewRecorder()
			strategy(got, http.StatusOK, value)

			if got.Code != want.Code {
				t.Fatalf("status = %d, implementação atual = %d", got.Code, want.Code)
			}
			if got.Body.String() != want.Body.String() {
				t.Fatalf("corpo de erro = %q, implementação atual = %q", got.Body.String(), want.Body.String())
			}
		})
	}
}

// BenchmarkJSONResponse compara a implementação atual com o candidato. O DTO
// é montado antes da medição para que ns/op, B/op e allocs/op representem
// somente a escrita da resposta JSON.
func BenchmarkJSONResponse(b *testing.B) {
	response := toOrderResponse(newOrderForMapper(b, 500))

	b.Run("json.Marshal", func(b *testing.B) {
		writer := newBenchmarkResponseWriter()
		b.ReportAllocs()
		for b.Loop() {
			writer.reset()
			writeJSON(writer, http.StatusOK, response)
		}
		benchmarkResponseSizeSink = writer.size
	})

	b.Run("new bytes.Buffer", func(b *testing.B) {
		writer := newBenchmarkResponseWriter()
		b.ReportAllocs()
		for b.Loop() {
			writer.reset()
			writeJSONWithNewBuffer(writer, http.StatusOK, response)
		}
		benchmarkResponseSizeSink = writer.size
	})

	b.Run("sync.Pool", func(b *testing.B) {
		writer := newBenchmarkResponseWriter()
		b.ReportAllocs()
		for b.Loop() {
			writer.reset()
			writeJSONWithPool(writer, http.StatusOK, response)
		}
		benchmarkResponseSizeSink = writer.size
	})
}

// benchmarkResponseWriter descarta o corpo sem alocá-lo. Assim o benchmark
// mede as estratégias de serialização, não o crescimento de um Recorder.
type benchmarkResponseWriter struct {
	header http.Header
	status int
	size   int
}

func newBenchmarkResponseWriter() *benchmarkResponseWriter {
	return &benchmarkResponseWriter{header: make(http.Header)}
}

func (writer *benchmarkResponseWriter) Header() http.Header {
	return writer.header
}

func (writer *benchmarkResponseWriter) WriteHeader(status int) {
	writer.status = status
}

func (writer *benchmarkResponseWriter) Write(payload []byte) (int, error) {
	writer.size += len(payload)
	return len(payload), nil
}

func (writer *benchmarkResponseWriter) reset() {
	clear(writer.header)
	writer.status = 0
	writer.size = 0
}
