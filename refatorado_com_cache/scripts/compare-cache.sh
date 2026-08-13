#!/usr/bin/env bash

set -euo pipefail

items="${ITEMS:-100}"
requests="${REQUESTS:-50}"
api_url="${API_URL:-http://127.0.0.1:8084}"
binary="$(mktemp)"
api_pid=""

cleanup() {
	if [[ -n "$api_pid" ]]; then
		kill "$api_pid" 2>/dev/null || true
		wait "$api_pid" 2>/dev/null || true
	fi
	rm -f "$binary"
}
trap cleanup EXIT

wait_for_api() {
	for _ in {1..50}; do
		if curl -fsS "$api_url/metrics/cache" >/dev/null 2>&1; then
			return 0
		fi
		sleep 0.1
	done
	return 1
}

stop_api() {
	kill "$api_pid"
	wait "$api_pid" 2>/dev/null || true
	api_pid=""
}

docker compose up -d --wait
go build -o "$binary" ./cmd/api

printf '\nLINHA DE BASE — MESMA API, CACHE DESATIVADO\n'
CACHE_ENABLED=false "$binary" >/tmp/aula-cache-baseline.log 2>&1 &
api_pid=$!
wait_for_api
go run ./cmd/cachelab -items "$items" -warm-requests "$requests" -api-url "$api_url"
stop_api

docker compose exec -T redis redis-cli FLUSHDB >/dev/null

printf '\nCACHE-ASIDE — MESMA CARGA, REDIS ATIVADO\n'
"$binary" >/tmp/aula-cache-enabled.log 2>&1 &
api_pid=$!
wait_for_api
go run ./cmd/cachelab -items "$items" -warm-requests "$requests" -api-url "$api_url"
stop_api

printf '\nOs contêineres permanecem ativos para inspecionar chave, TTL e métricas.\n'
