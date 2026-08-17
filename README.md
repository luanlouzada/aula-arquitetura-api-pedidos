# API de pedidos: do legado aos logs estruturados

Este repositório apresenta **uma única API** em três versões:

- `legado`: HTTP, JSON, regras de negócio, transações e SQL estão concentrados no mesmo componente;
- `refatorado`: o contrato HTTP permanece igual, mas o código é separado em domínio, aplicação, camada HTTP e infraestrutura;
- `refatorado_com_slog`: uma cópia do refatorado com configuração e logs estruturados em JSON usando apenas a biblioteca padrão do Go.

O repositório também possui laboratórios independentes usados nas aulas de
concorrência e operação:

- [`aula-fila-worker-pool`](aula-fila-worker-pool/README.md): referência de fila limitada e worker pool;
- [`exercicio`](exercicio/README.md): exercício introdutório da cozinha, com TODOs de fila e workers;
- [`aula-08-confiabilidade`](aula-08-confiabilidade/README.md): referência completa com token bucket, NGINX, três instâncias, health checks, métricas e graceful shutdown;
- [`exercicio-aula-08`](exercicio-aula-08/README.md): exercício equivalente no domínio de exportação de relatórios, com TODOs e testes orientadores.

Legado e refatorado usam o PostgreSQL do `compose.yaml` da raiz. A versão com `slog` possui um Compose independente e pode ser executada isoladamente.

## Contrato da API

| Método | Rota | Comportamento |
| --- | --- | --- |
| `POST` | `/orders` | Cria um pedido pendente |
| `GET` | `/orders/{id}` | Consulta um pedido |
| `PATCH` | `/orders/{id}/pay` | Paga um pedido pendente |
| `PATCH` | `/orders/{id}/cancel` | Cancela um pedido pendente |

Exemplo de criação:

```json
{
  "cliente": "Ana",
  "itens": [
    {
      "produto": "Notebook",
      "preco_unitario_centavos": 450000,
      "quantidade": 2
    }
  ]
}
```

Estados permitidos:

```text
                pagar
PENDENTE -----------------> PAGO
    |
    | cancelar
    v
CANCELADO
```

Pedidos pagos ou cancelados não podem mudar novamente de estado.

## Executar o PostgreSQL

```bash
docker compose up -d --wait
```

O banco fica disponível em `localhost:5435`, database `pedidos`, usuário e senha `postgres`.

> O script `database/init.sql` é executado somente quando o volume está vazio. Para reconstruir deliberadamente o banco local: `docker compose down -v` e depois `docker compose up -d --wait`.

## Executar o legado

```bash
go run ./legado/cmd/api
```

A API legado usa `http://127.0.0.1:8080`.

Em outro terminal:

```bash
API_BASE_URL=http://127.0.0.1:8080 go test ./contract-tests -count=1 -v
```

## Executar a versão refatorada

```bash
go run ./refatorado/cmd/api
```

A API refatorada usa `http://127.0.0.1:8081`.

Em outro terminal:

```bash
API_BASE_URL=http://127.0.0.1:8081 go test ./contract-tests -count=1 -v
```

## Executar a versão refatorada com slog

```bash
cd refatorado_com_slog
docker compose up -d --wait
go run ./cmd/api
```

Essa versão usa:

- API em `http://127.0.0.1:8083`;
- PostgreSQL em `localhost:5437`.

Em outro terminal, a partir da raiz:

```bash
API_BASE_URL=http://127.0.0.1:8083 go test ./contract-tests -count=1 -v
```

Os detalhes de execução estão em `refatorado_com_slog/README.md`.

## Configuração

As duas versões aceitam:

| Variável | Legado | Refatorado | Refatorado com slog |
| --- | --- | --- | --- |
| `DATABASE_URL` | PostgreSQL em `5435` | igual | PostgreSQL em `5437` |
| `API_ADDR` | `127.0.0.1:8080` | `127.0.0.1:8081` | `127.0.0.1:8083` |

A versão com logs também aceita `SERVICE_NAME`, `APP_ENV` e `LOG_LEVEL`.

## Comparação arquitetural

### Antes

```text
request HTTP
     ↓
handler
 ├── decodifica JSON
 ├── valida regra de negócio
 ├── inicia transação
 ├── executa SQL
 ├── traduz linhas do PostgreSQL
 ├── decide status HTTP
 └── serializa resposta
```

O código funciona, mas uma única unidade muda por razões diferentes: protocolo, negócio e persistência.

### Depois

```text
HTTP request
     ↓
Controller + DTO + Mapper       camada HTTP
     ↓
OrderService                    application
     ↓
Order                           domain
     ↓ usa
OrderRepository                 interface exigida pela aplicação
     ↑ implementada por
PostgresOrderRepository         implementação concreta
     ↓
PostgreSQL                      infrastructure
```

O `cmd/api/main.go` é o composition root: cria o pool, o Repository, a Service e o Controller.

## Testes

Testes rápidos da versão refatorada, sem PostgreSQL:

```bash
go test ./refatorado/... -count=1
```

Testes rápidos da versão com logs estruturados:

```bash
go test ./refatorado_com_slog/... -count=1
```

A suíte de contrato atravessa a API real e o PostgreSQL. Ela é compartilhada pelas três versões para verificar que a refatoração preservou o comportamento da API.
