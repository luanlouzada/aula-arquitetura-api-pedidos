# Laboratório de fila limitada e worker pool

O processo aceita trabalhos por HTTP, coloca-os em um `chan` com capacidade fixa e os entrega a uma quantidade fixa de workers. Quando a fila enche, novas requisições recebem `503 Service Unavailable` e `Retry-After`.

```bash
make test
make run
```

Em outro terminal:

```bash
make load-small
make stats
make load-overload
make stats
```

As duas cargas abaixo contrastam um ritmo sustentado abaixo e acima da capacidade padrão aproximada de 40 trabalhos por segundo:

```bash
make load-steady-ok
make load-steady-overload
```

Para comparar processamento unitário com lote, encerre a primeira execução e inicie:

```bash
make run-batch
```

O `chan` pertence a um único processo e perde os itens quando ele encerra. Um trabalho que precisa sobreviver a reinícios exige uma fila durável, confirmação de publicação, acknowledgements e política de redelivery ou dead letter.
