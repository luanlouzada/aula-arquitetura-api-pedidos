.PHONY: run load-small load-overload load-steady-ok load-steady-overload test

run:
	go run ./cmd/api

load-small:
	go run ./cmd/load -requests=12 -concurrency=2

load-overload:
	go run ./cmd/load -requests=200 -concurrency=50

load-steady-ok:
	go run ./cmd/load -requests=120 -concurrency=10 -rate=30

load-steady-overload:
	go run ./cmd/load -requests=240 -concurrency=20 -rate=60

test:
	go test ./...
