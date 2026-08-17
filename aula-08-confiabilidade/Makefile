.PHONY: run test race vet lab-up lab-down lab-reset load-burst load-steady load-overload stats drain-api-2

run:
	INSTANCE_ID=local API_ADDR=127.0.0.1:8080 go run ./cmd/api

test:
	go test ./... -count=1

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

load-burst:
	go run ./cmd/load -requests=60 -concurrency=20

load-steady:
	go run ./cmd/load -requests=60 -concurrency=10 -rate=12

load-overload:
	go run ./cmd/load -requests=120 -concurrency=30 -rate=60

stats:
	@for port in 8081 8082 8083; do curl -sS "http://127.0.0.1:$$port/stats"; echo; done

drain-api-2:
	./scripts/drain-instance.sh api-2
