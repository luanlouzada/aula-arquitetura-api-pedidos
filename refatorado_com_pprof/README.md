# API de pedidos com pprof

Esta variante parte de `refatorado` e acrescenta um laboratório de CPU e memória sobre a API real. O foco permanece em profiling: a aplicação conserva as mesmas regras, rotas e separação de camadas da base refatorada.

O processo possui duas interfaces:

| Endereço | Responsabilidade |
| --- | --- |
| `127.0.0.1:8083` | API pública com as rotas de pedidos |
| `127.0.0.1:6060` | interface administrativa `/debug/pprof/` |

O endereço administrativo usa loopback por padrão. Ele não deve ser publicado diretamente na internet, pois os perfis revelam detalhes internos do processo e ainda acrescentam custo durante a coleta.

## O problema investigado

`GET /orders/{id}` executa este caminho:

```text
cliente HTTP
    → Controller
    → OrderService
    → Repository PostgreSQL
    → domain.Order
    → mapper HTTP
    → encoding/json
```

`Order.Items()` protege o agregado devolvendo uma cópia do slice interno. O mapper inicial chama esse getter novamente para cada item:

```go
orderItems := order.Items()
items := make([]orderItemResponse, 0, len(orderItems))
for index := range orderItems {
    item := order.Items()[index]
    // converte item para o DTO HTTP
}
```

O JSON está correto, mas um pedido com 400 ou 500 itens provoca centenas de cópias. O custo cresce muito mais rápido do que a quantidade de itens.

O teste protege o contrato HTTP. O benchmark mede o mapper isoladamente. A carga HTTP permite confirmar a mesma causa dentro da aplicação em execução.

## Arquivos acrescentados

```text
refatorado_com_pprof/
├── cmd/
│   ├── api/
│   │   ├── main.go
│   │   ├── pprof.go
│   │   └── pprof_test.go
│   └── load/
│       └── main.go
├── internal/
│   ├── config/
│   │   ├── config.go
│   │   └── config_test.go
│   └── httpapi/
│       ├── mapper.go
│       └── mapper_test.go
└── profiles/                 # criado pelos comandos de coleta
```

`cmd/load` não é uma rota especial nem um atalho para o domínio. Ele é um cliente da API: cria um pedido com `POST /orders` e consulta o pedido com `GET /orders/{id}`.

## Executar

```bash
cd refatorado_com_pprof
docker compose up -d --wait
go run ./cmd/api
```

Confirmação das duas interfaces:

```bash
curl -i http://127.0.0.1:6060/debug/pprof/
curl -i http://127.0.0.1:8083/debug/pprof/
```

O primeiro comando deve responder `200`. O segundo deve responder `404`, pois a API pública não expõe os handlers de profiling.

## Criar a linha de base

```bash
go test ./internal/httpapi -run '^$' \
  -bench '^BenchmarkToOrderResponse$' \
  -benchmem \
  -count=5
```

Uma execução no ambiente usado para validar o projeto produziu aproximadamente:

```text
2,1 ms/op    8,2 MB/op    502 allocs/op
```

Os números absolutos variam por máquina. O que precisa permanecer igual na comparação é a entrada, a versão do Go, o ambiente e o comportamento verificado pelo teste.

## Coletar CPU durante carga HTTP

Crie a pasta que receberá os perfis:

```bash
mkdir -p profiles
```

Com a API em execução, inicie a carga em outro terminal:

```bash
go run ./cmd/load \
  -items 400 \
  -concurrency 4 \
  -duration 35s
```

Enquanto a carga estiver ativa, colete trinta segundos de CPU:

```bash
curl -fsS \
  -o profiles/cpu-before.pprof \
  'http://127.0.0.1:6060/debug/pprof/profile?seconds=30'
```

Leitura inicial:

```bash
go tool pprof -top -cum profiles/cpu-before.pprof
go tool pprof -list 'toOrderResponse' profiles/cpu-before.pprof
go tool pprof -http=127.0.0.1:0 profiles/cpu-before.pprof
```

Na validação do projeto, `toOrderResponse` acumulou cerca de 41% do tempo de CPU. A listagem por linha apontou para:

```go
item := order.Items()[index]
```

Rotinas como `runtime.memmove`, criação de slices e garbage collector aparecem porque copiar a estrutura exige alocar memória, mover os itens e posteriormente tornar reutilizável o espaço dos objetos temporários.

## Coletar alocações

Enquanto uma nova carga de pelo menos 35 segundos estiver ativa, colete uma
janela de trinta segundos:

```bash
curl -fsS \
  -o profiles/allocs-before.pprof \
  'http://127.0.0.1:6060/debug/pprof/allocs?seconds=30'

go tool pprof \
  -top \
  -alloc_space \
  profiles/allocs-before.pprof
```

`alloc_space` mostra os bytes alocados ao longo da execução, inclusive os bytes de objetos cujo espaço o garbage collector já tornou reutilizável. Na validação, `domain.copyItems` respondeu por aproximadamente 95% dos bytes acumulados durante a carga.

## Corrigir uma única causa

`Order.Items()` continua devolvendo uma cópia defensiva. A correção consiste em reutilizar essa única cópia durante o mapeamento:

```go
func toOrderResponse(order domain.Order) orderResponse {
    orderItems := order.Items()
    items := make([]orderItemResponse, 0, len(orderItems))
    for _, item := range orderItems {
        items = append(items, orderItemResponse{
            ProductName:    item.ProductName(),
            UnitPriceCents: item.UnitPriceCents(),
            Quantity:       item.Quantity(),
            SubtotalCents:  item.SubtotalCents(),
        })
    }

    return orderResponse{
        ID:         order.ID(),
        Customer:   order.Customer(),
        Status:     string(order.Status()),
        TotalCents: order.TotalCents(),
        Version:    order.Version(),
        CreatedAt:  order.CreatedAt(),
        Items:      items,
    }
}
```

O domínio não perde a cópia defensiva e o contrato JSON não muda. Somente as cópias repetidas desaparecem.

## Verificar o depois

```bash
go test ./... -count=1
go test -race ./...

go test ./internal/httpapi -run '^$' \
  -bench '^BenchmarkToOrderResponse$' \
  -benchmem \
  -count=5
```

A correção foi medida separadamente em uma cópia de validação e produziu aproximadamente:

```text
10,9 µs/op    36 KB/op    2 allocs/op
```

Reinicie a API com o código corrigido para não misturar o histórico anterior.
Em seguida, repita a carga e salve novos perfis:

```bash
go run ./cmd/load -items 400 -concurrency 4 -duration 35s

curl -fsS \
  -o profiles/cpu-after.pprof \
  'http://127.0.0.1:6060/debug/pprof/profile?seconds=30'

curl -fsS \
  -o profiles/allocs-after.pprof \
  'http://127.0.0.1:6060/debug/pprof/allocs?seconds=30'
```

O teste responde se o resultado continua correto. O benchmark responde quanto o trecho isolado mudou. O novo perfil responde se a causa deixou de dominar o processo sob carga HTTP.

## Verificação do projeto

```bash
go test ./... -count=1
go vet ./...
docker compose config
```

Para encerrar o PostgreSQL sem apagar o volume:

```bash
docker compose down
```
