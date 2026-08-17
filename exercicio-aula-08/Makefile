.PHONY: run test test-token test-health test-http test-shutdown test-nginx race vet lab-up lab-down lab-reset load stats drain-exporter-2

run:
	INSTANCE_ID=local go run ./cmd/api

test:
	go test ./... -count=1

test-token:
	go test ./internal/admission -v -count=1

test-health:
	go test ./internal/health -v -count=1

test-http:
	go test ./internal/httpapi -v -count=1

test-shutdown:
	go test ./internal/lifecycle -v -count=1

test-nginx:
	go test ./internal/labconfig -v -count=1

race:
	go test -race ./... -count=1

vet:
	go vet ./...

lab-up:
	docker compose up --build -d --wait

lab-down:
	docker compose down

lab-reset:
	docker compose down
	docker compose up --build -d --wait

load:
	go run ./cmd/load -requests=60 -concurrency=10 -rate=12

stats:
	@for port in 8091 8092 8093; do curl -sS "http://127.0.0.1:$$port/stats"; echo; done

drain-exporter-2:
	./scripts/drain-instance.sh exporter-2
