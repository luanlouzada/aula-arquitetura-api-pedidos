SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

API_URL ?= http://127.0.0.1:8083
PPROF_URL ?= http://127.0.0.1:6060
PPROF_HTTP ?= 127.0.0.1:0

ITEMS ?= 400
CONCURRENCY ?= 4
LOAD_DURATION ?= 35s
PROFILE_SECONDS ?= 30
FUNCTION ?= toOrderResponse
LABEL ?= before

SCENARIO ?= cpu
LAB_PPROF_ADDR ?= 127.0.0.1:6061
LAB_DURATION ?= 60s
MEMORY_MB ?= 64
GOROUTINES ?= 1000

PROFILE_DIR ?= profiles
CPU_PROFILE = $(PROFILE_DIR)/cpu-$(LABEL).pprof
ALLOCS_PROFILE = $(PROFILE_DIR)/allocs-$(LABEL).pprof
GOROUTINE_PROFILE = $(PROFILE_DIR)/goroutine.pprof
CPU_LOAD_LOG = $(PROFILE_DIR)/load-cpu-$(LABEL).log
ALLOCS_LOAD_LOG = $(PROFILE_DIR)/load-allocs-$(LABEL).log
BENCH_RESULT = $(PROFILE_DIR)/bench-$(LABEL).txt

.PHONY: help db-up run stop status check-pprof load bench benchstat \
	cpu cpu-profile cpu-top cpu-indices cpu-list cpu-web \
	allocs allocs-profile allocs-top \
	lab lab-cpu lab-io lab-goroutine lab-memory lab-allocs lab-mutex lab-block \
	goroutine-profile goroutine-stacks pool-bench escape-analysis \
	before after test test-race verify down

help:
	@printf '%s\n' \
		'Laboratório de pprof da API de pedidos' \
		'' \
		'Fluxo curto, usando dois terminais:' \
		'  terminal 1: make run' \
		'  terminal 2: make status && make before' \
		'  depois da correção: make after' \
		'' \
		'Alvos principais:' \
		'  make run          sobe o PostgreSQL e inicia a API' \
		'  make stop         encerra a API e o pprof nas portas 8083 e 6060' \
		'  make status       confirma pprof=200 e API pública=404' \
		'  make bench        mede o mapper e salva o resultado' \
		'  make cpu          gera CPU profile, top -cum e list' \
		'  make cpu-indices  compara os índices samples e cpu' \
		'  make cpu-web      abre a interface visual do CPU profile' \
		'  make allocs       gera allocs profile e mostra alloc_space' \
		'  make pool-bench   compara a escrita JSON atual com sync.Pool' \
		'  make escape-analysis mostra decisões do compilador no pacote HTTP' \
		'  make before       executa benchmark, CPU e alocações' \
		'  make after        repete as coletas com LABEL=after' \
		'  make benchstat    compara bench-before.txt e bench-after.txt' \
		'  make lab-cpu      executa CPU isoladamente, sem a API' \
		'  make lab-io       executa I/O isoladamente, sem a API' \
		'  make lab-memory   retém memória para o heap profile' \
		'  make lab-allocs   cria churn para o allocs profile' \
		'  make lab-goroutine cria goroutines estacionadas' \
		'  make goroutine-profile coleta o snapshot binário do laboratório' \
		'  make goroutine-stacks mostra stacks agrupadas do laboratório' \
		'  make lab-mutex    cria contenção de mutex' \
		'  make lab-block    cria bloqueio em canal' \
		'  make test         executa os testes' \
		'  make verify       executa testes, vet e valida o Compose' \
		'  make down         encerra o PostgreSQL sem apagar o volume' \
		'' \
		'Variáveis opcionais:' \
		'  ITEMS=400 CONCURRENCY=4 LOAD_DURATION=35s' \
		'  PROFILE_SECONDS=30 LABEL=before' \
		'  SCENARIO=cpu LAB_DURATION=60s MEMORY_MB=64 GOROUTINES=1000' \
		'' \
		'Exemplo rápido: make cpu PROFILE_SECONDS=10 LOAD_DURATION=15s'

db-up:
	docker compose up -d --wait

run: db-up
	@printf 'API:   %s\npprof: %s/debug/pprof/\n\n' "$(API_URL)" "$(PPROF_URL)"
	go run ./cmd/api

stop:
	@command -v fuser >/dev/null 2>&1 || { \
		printf 'fuser não está instalado; instale o pacote psmisc.\n' >&2; \
		exit 1; \
	}
	@listeners="$$(fuser 8083/tcp 6060/tcp 2>/dev/null || true)"; \
	if [[ -z "$$listeners" ]]; then \
		printf 'API e pprof já estão parados.\n'; \
		exit 0; \
	fi; \
	printf 'Encerrando API e pprof (PIDs:%s)...\n' "$$listeners"; \
	fuser -k -TERM 8083/tcp 6060/tcp >/dev/null 2>&1 || true; \
	for _ in $$(seq 1 50); do \
		if ! fuser 8083/tcp 6060/tcp >/dev/null 2>&1; then \
			printf 'API e pprof encerrados.\n'; \
			exit 0; \
		fi; \
		sleep 0.1; \
	done; \
	printf 'As portas 8083 ou 6060 continuam ocupadas:\n' >&2; \
	fuser -v 8083/tcp 6060/tcp >&2 || true; \
	exit 1

status:
	@pprof_code="$$(curl -sS -o /dev/null -w '%{http_code}' "$(PPROF_URL)/debug/pprof/" || true)"; \
	api_code="$$(curl -sS -o /dev/null -w '%{http_code}' "$(API_URL)/debug/pprof/" || true)"; \
	printf 'pprof administrativo: %s (esperado 200)\n' "$$pprof_code"; \
	printf 'pprof na API pública: %s (esperado 404)\n' "$$api_code"; \
	if [[ "$$pprof_code" != 200 || "$$api_code" != 404 ]]; then \
		printf '\nA API não está pronta ou as fronteiras não estão corretas. Execute make run em outro terminal.\n' >&2; \
		exit 1; \
	fi

check-pprof:
	@code="$$(curl -sS -o /dev/null -w '%{http_code}' "$(PPROF_URL)/debug/pprof/" || true)"; \
	if [[ "$$code" != 200 ]]; then \
		printf 'pprof respondeu %s; execute make run em outro terminal.\n' "$$code" >&2; \
		exit 1; \
	fi

load:
	go run ./cmd/load \
		-items "$(ITEMS)" \
		-concurrency "$(CONCURRENCY)" \
		-duration "$(LOAD_DURATION)"

bench:
	@mkdir -p "$(PROFILE_DIR)"
	go test ./internal/httpapi -run '^$$' \
		-bench '^BenchmarkToOrderResponse$$' \
		-benchmem \
		-count=5 | tee "$(BENCH_RESULT)"
	@printf '\nResultado salvo em %s\n' "$(BENCH_RESULT)"

benchstat:
	@test -f "$(PROFILE_DIR)/bench-before.txt" || { printf 'Falta profiles/bench-before.txt\n' >&2; exit 1; }
	@test -f "$(PROFILE_DIR)/bench-after.txt" || { printf 'Falta profiles/bench-after.txt\n' >&2; exit 1; }
	go run golang.org/x/perf/cmd/benchstat@latest \
		"$(PROFILE_DIR)/bench-before.txt" \
		"$(PROFILE_DIR)/bench-after.txt"

cpu:
	@$(MAKE) --no-print-directory cpu-profile
	@$(MAKE) --no-print-directory cpu-top
	@$(MAKE) --no-print-directory cpu-list

cpu-profile: check-pprof
	@mkdir -p "$(PROFILE_DIR)"
	@printf 'Carga: %s itens, concorrência %s, duração %s\n' "$(ITEMS)" "$(CONCURRENCY)" "$(LOAD_DURATION)"
	@printf 'Coleta: %s segundos de CPU -> %s\n\n' "$(PROFILE_SECONDS)" "$(CPU_PROFILE)"
	@log="$(CPU_LOAD_LOG)"; \
	profile="$(CPU_PROFILE)"; \
	: > "$$log"; \
	go run ./cmd/load -items "$(ITEMS)" -concurrency "$(CONCURRENCY)" -duration "$(LOAD_DURATION)" > "$$log" 2>&1 & \
	load_pid=$$!; \
	trap 'kill "$$load_pid" 2>/dev/null || true; wait "$$load_pid" 2>/dev/null || true' EXIT INT TERM; \
	ready=0; \
	for _ in $$(seq 1 200); do \
		if grep -q 'consultando GET' "$$log"; then ready=1; break; fi; \
		if ! kill -0 "$$load_pid" 2>/dev/null; then cat "$$log"; wait "$$load_pid"; exit 1; fi; \
		sleep 0.1; \
	done; \
	if [[ "$$ready" != 1 ]]; then cat "$$log"; printf 'A carga não ficou pronta.\n' >&2; exit 1; fi; \
	curl -fsS -o "$$profile" "$(PPROF_URL)/debug/pprof/profile?seconds=$(PROFILE_SECONDS)"; \
	if wait "$$load_pid"; then load_status=0; else load_status=$$?; fi; \
	trap - EXIT INT TERM; \
	cat "$$log"; \
	if [[ "$$load_status" != 0 ]]; then exit "$$load_status"; fi; \
	printf 'CPU profile salvo em %s\n' "$$profile"

cpu-top:
	@test -f "$(CPU_PROFILE)" || { printf 'Falta %s; execute make cpu-profile.\n' "$(CPU_PROFILE)" >&2; exit 1; }
	go tool pprof -top -cum "$(CPU_PROFILE)"

cpu-indices:
	@test -f "$(CPU_PROFILE)" || { printf 'Falta %s; execute make cpu-profile.\n' "$(CPU_PROFILE)" >&2; exit 1; }
	@printf 'Mesmo perfil contado por amostras:\n\n'
	go tool pprof -top -sample_index=samples "$(CPU_PROFILE)"
	@printf '\nMesmo perfil convertido em tempo de CPU:\n\n'
	go tool pprof -top -sample_index=cpu "$(CPU_PROFILE)"

cpu-list:
	@test -f "$(CPU_PROFILE)" || { printf 'Falta %s; execute make cpu-profile.\n' "$(CPU_PROFILE)" >&2; exit 1; }
	go tool pprof -list "$(FUNCTION)" "$(CPU_PROFILE)"

cpu-web:
	@test -f "$(CPU_PROFILE)" || { printf 'Falta %s; execute make cpu.\n' "$(CPU_PROFILE)" >&2; exit 1; }
	go tool pprof -http="$(PPROF_HTTP)" "$(CPU_PROFILE)"

allocs:
	@$(MAKE) --no-print-directory allocs-profile
	@$(MAKE) --no-print-directory allocs-top

allocs-profile: check-pprof
	@mkdir -p "$(PROFILE_DIR)"
	@printf 'Carga: %s itens, concorrência %s, duração %s\n' "$(ITEMS)" "$(CONCURRENCY)" "$(LOAD_DURATION)"
	@printf 'Coleta: %s segundos de alocações -> %s\n\n' "$(PROFILE_SECONDS)" "$(ALLOCS_PROFILE)"
	@log="$(ALLOCS_LOAD_LOG)"; \
	profile="$(ALLOCS_PROFILE)"; \
	: > "$$log"; \
	go run ./cmd/load -items "$(ITEMS)" -concurrency "$(CONCURRENCY)" -duration "$(LOAD_DURATION)" > "$$log" 2>&1 & \
	load_pid=$$!; \
	trap 'kill "$$load_pid" 2>/dev/null || true; wait "$$load_pid" 2>/dev/null || true' EXIT INT TERM; \
	ready=0; \
	for _ in $$(seq 1 200); do \
		if grep -q 'consultando GET' "$$log"; then ready=1; break; fi; \
		if ! kill -0 "$$load_pid" 2>/dev/null; then cat "$$log"; wait "$$load_pid"; exit 1; fi; \
		sleep 0.1; \
	done; \
	if [[ "$$ready" != 1 ]]; then cat "$$log"; printf 'A carga não ficou pronta.\n' >&2; exit 1; fi; \
	curl -fsS -o "$$profile" "$(PPROF_URL)/debug/pprof/allocs?seconds=$(PROFILE_SECONDS)"; \
	if wait "$$load_pid"; then load_status=0; else load_status=$$?; fi; \
	trap - EXIT INT TERM; \
	cat "$$log"; \
	if [[ "$$load_status" != 0 ]]; then exit "$$load_status"; fi; \
	printf 'Perfil de alocações salvo em %s\n' "$$profile"

allocs-top:
	@test -f "$(ALLOCS_PROFILE)" || { printf 'Falta %s; execute make allocs-profile.\n' "$(ALLOCS_PROFILE)" >&2; exit 1; }
	go tool pprof -top -sample_index=alloc_space "$(ALLOCS_PROFILE)"

lab:
	go run ./cmd/pproflab \
		-scenario "$(SCENARIO)" \
		-pprof-addr "$(LAB_PPROF_ADDR)" \
		-duration "$(LAB_DURATION)" \
		-memory-mb "$(MEMORY_MB)" \
		-goroutines "$(GOROUTINES)"

lab-cpu:
	@$(MAKE) --no-print-directory lab SCENARIO=cpu

lab-io:
	@$(MAKE) --no-print-directory lab SCENARIO=io

lab-goroutine:
	@$(MAKE) --no-print-directory lab SCENARIO=goroutine

lab-memory:
	@$(MAKE) --no-print-directory lab SCENARIO=memory

lab-allocs:
	@$(MAKE) --no-print-directory lab SCENARIO=allocs

lab-mutex:
	@$(MAKE) --no-print-directory lab SCENARIO=mutex

lab-block:
	@$(MAKE) --no-print-directory lab SCENARIO=block

goroutine-profile:
	@mkdir -p "$(PROFILE_DIR)"
	curl -fsS -o "$(GOROUTINE_PROFILE)" \
		"http://$(LAB_PPROF_ADDR)/debug/pprof/goroutine"
	go tool pprof -top "$(GOROUTINE_PROFILE)"

goroutine-stacks:
	curl -fsS "http://$(LAB_PPROF_ADDR)/debug/pprof/goroutine?debug=1"

pool-bench:
	go test ./internal/httpapi \
		-run '^TestWriteJSON' \
		-bench '^BenchmarkJSONResponse$$' \
		-benchmem \
		-count=5

escape-analysis:
	go test -run '^$$' \
		-gcflags='aula-pedidos/refatorado_com_pprof/internal/httpapi=-m=2' \
		./internal/httpapi 2>&1 | \
		rg 'controller\.go:.*(append\(payload|parameter value leaks)|mapper\.go:.*make\(\[\](domain\.Item|orderItemResponse)'

before:
	@$(MAKE) --no-print-directory bench LABEL=before
	@$(MAKE) --no-print-directory cpu LABEL=before
	@$(MAKE) --no-print-directory allocs LABEL=before

after:
	@$(MAKE) --no-print-directory bench LABEL=after
	@$(MAKE) --no-print-directory cpu LABEL=after
	@$(MAKE) --no-print-directory allocs LABEL=after

test:
	go test ./... -count=1

test-race:
	go test -race ./...

verify:
	go test ./... -count=1
	go vet ./...
	docker compose config --quiet

down:
	docker compose down
