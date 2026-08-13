# API de pedidos refatorada com slog

Esta versão acrescenta logs estruturados à API de pedidos com o pacote `log/slog`, que faz parte da biblioteca padrão do Go.

As responsabilidades de domínio, aplicação, HTTP e PostgreSQL permanecem separadas. Logging é acrescentado nas bordas apropriadas sem colocar chamadas de log dentro da entidade `Order`.

## Componentes

| Componente | Responsabilidade |
| --- | --- |
| `internal/config` | lê `LOG_LEVEL`, `SERVICE_NAME` e `APP_ENV` |
| `internal/logging.NewLogger` | cria o logger JSON e adiciona campos comuns |
| `internal/httpapi.AccessLog` | registra uma linha ao final de cada requisição |
| `internal/logging.LoggedOrderService` | registra mudanças de negócio concluídas |
| `internal/httpapi.Controller` | registra falhas técnicas inesperadas |
| `cmd/api` | monta os componentes e injeta o mesmo logger |

## Fluxo

```text
requisição HTTP
      │
      ▼
AccessLog inicia a medição
      │
      ▼
Controller
      │
      ▼
LoggedOrderService
      │
      ▼
OrderService → domínio → Repository PostgreSQL
      │
      ▼
LoggedOrderService registra a mudança concluída
      │
      ▼
AccessLog registra status, bytes e duração
```

## Configuração

| Variável | Valor padrão |
| --- | --- |
| `API_ADDR` | `127.0.0.1:8083` |
| `DATABASE_URL` | PostgreSQL em `localhost:5437` |
| `SERVICE_NAME` | `pedidos-api` |
| `APP_ENV` | `development` |
| `LOG_LEVEL` | `info` |

Níveis aceitos em `LOG_LEVEL`: `debug`, `info`, `warn` e `error`.

O arquivo `.env.example` documenta os valores locais. O programa não carrega esse arquivo automaticamente. Se quiser utilizá-lo no terminal:

```bash
set -a
source .env.example
set +a
```

## Executar

```bash
cd refatorado_com_slog
docker compose up -d --wait
go run ./cmd/api
```

Endereços:

- API: `http://127.0.0.1:8083`;
- PostgreSQL: `localhost:5437`.

## Gerar logs

```bash
curl -i -X POST http://localhost:8083/orders \
  -H 'Content-Type: application/json' \
  -d '{
    "cliente": "Ana",
    "itens": [
      {
        "produto": "Notebook",
        "preco_unitario_centavos": 450000,
        "quantidade": 2
      }
    ]
  }'

curl -i -X PATCH http://localhost:8083/orders/1/pay
```

Cada linha escrita no terminal é um objeto JSON independente. Um evento de negócio possui campos como:

```json
{
  "level": "INFO",
  "msg": "pedido pago",
  "service": "pedidos-api",
  "environment": "development",
  "operation": "orders.pay",
  "order_id": 1,
  "order_status": "PAGO",
  "version": 2
}
```

O middleware também produz um registro da requisição:

```json
{
  "level": "INFO",
  "msg": "requisição concluída",
  "service": "pedidos-api",
  "environment": "development",
  "http.method": "PATCH",
  "http.route": "/orders/{id}/pay",
  "http.status_code": 200,
  "http.response_bytes": 181,
  "duration_ms": 4
}
```

## Verificar

```bash
go test ./... -count=1
go vet ./...
docker compose config
```

Para encerrar a infraestrutura sem apagar o volume do PostgreSQL:

```bash
docker compose down
```
