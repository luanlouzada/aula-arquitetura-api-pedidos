.PHONY: run run-batch load-small load-overload load-steady-ok load-steady-overload stats test

run:
	go run ./cmd/api

run-batch:
	BATCH_SIZE=10 BATCH_WAIT=50ms go run ./cmd/api

load-small:
	go run ./cmd/load -requests=12 -concurrency=2

load-overload:
	go run ./cmd/load -requests=200 -concurrency=50

load-steady-ok:
	go run ./cmd/load -requests=120 -concurrency=10 -rate=30

load-steady-overload:
	go run ./cmd/load -requests=240 -concurrency=20 -rate=60

stats:
	curl -sS http://127.0.0.1:8087/stats

test:
	go test ./...
