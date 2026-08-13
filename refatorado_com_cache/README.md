# API de pedidos com cache-aside

Esta versão parte de `refatorado` e acrescenta Redis somente à leitura
`GET /orders/{id}`. PostgreSQL continua sendo a fonte de verdade.

```bash
make up
make run
```

A API usa `http://127.0.0.1:8084`, PostgreSQL usa a porta `5438` e Redis usa
`6380`. Em outro terminal, crie e consulte um pedido; depois observe:

```bash
make metrics
make key ORDER_ID=1
make ttl ORDER_ID=1
make load ORDER_ID=1 REQUESTS=20
```

As chaves seguem o formato `pedidos:orders:v1:{id}` e expiram em 30 segundos
por padrão. `PATCH /orders/{id}/pay` e `/cancel` persistem no PostgreSQL e
invalidam a chave; a próxima leitura repopula o cache.
