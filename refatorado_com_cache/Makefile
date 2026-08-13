SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

API_URL ?= http://127.0.0.1:8084
REDIS_CONTAINER ?= aula-pedidos-cache-redis-1
ORDER_ID ?= 1
REQUESTS ?= 20
ITEMS ?= 100

.PHONY: help up run run-baseline down status metrics key ttl clear test verify load demo compare

help:
	@printf '%s\n' \
		'Laboratório de cache da API de pedidos' \
		'' \
		'  make up       sobe PostgreSQL e Redis' \
		'  make run      inicia a API em outro terminal' \
		'  make run-baseline  inicia a mesma API sem cache para comparar' \
		'  make status   verifica API, PostgreSQL e Redis' \
		'  make metrics  mostra hits, misses, erros e hit ratio' \
		'  make key ORDER_ID=1  inspeciona o JSON no Redis' \
		'  make ttl ORDER_ID=1  mostra o TTL restante' \
		'  make clear    esvazia somente o Redis deste Compose' \
		'  make load ORDER_ID=1 REQUESTS=20  repete a leitura e mede o tempo' \
		'  make demo ITEMS=100 REQUESTS=50  compara miss e hits' \
		'  make compare ITEMS=100 REQUESTS=50  mede sem cache e com cache' \
		'  make test     executa os testes Go' \
		'  make down     encerra os contêineres sem apagar o volume'

up:
	docker compose up -d --wait

run: up
	go run ./cmd/api

run-baseline: up
	CACHE_ENABLED=false go run ./cmd/api

down:
	docker compose down

status:
	@docker compose ps
	@printf '\nRedis: '
	@docker compose exec -T redis redis-cli ping
	@printf 'API: '
	@curl -sS -o /dev/null -w '%{http_code}\n' "$(API_URL)/metrics/cache"

metrics:
	@curl -fsS "$(API_URL)/metrics/cache"; printf '\n'

key:
	@docker compose exec -T redis redis-cli GET "pedidos:orders:v1:$(ORDER_ID)"

ttl:
	@docker compose exec -T redis redis-cli TTL "pedidos:orders:v1:$(ORDER_ID)"

clear:
	@docker compose exec -T redis redis-cli FLUSHDB

load:
	@printf 'Lendo pedido %s, %s vezes:\n' "$(ORDER_ID)" "$(REQUESTS)"
	@for _ in $$(seq 1 "$(REQUESTS)"); do \
		curl -fsS -o /dev/null -w '%{time_total}\n' "$(API_URL)/orders/$(ORDER_ID)"; \
	done

demo:
	go run ./cmd/cachelab -items "$(ITEMS)" -warm-requests "$(REQUESTS)" -api-url "$(API_URL)"

compare:
	ITEMS="$(ITEMS)" REQUESTS="$(REQUESTS)" API_URL="$(API_URL)" bash ./scripts/compare-cache.sh

test:
	go test ./... -count=1

verify: test
	go vet ./...
	docker compose config -q
