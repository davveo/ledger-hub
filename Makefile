.PHONY: tidy build test api gateway worker connector docker-up docker-down

tidy:
	go mod tidy

test:
	go test ./...

build:
	mkdir -p bin
	go build -o bin/ledger-api ./cmd/api
	go build -o bin/ledger-gateway ./cmd/gateway
	go build -o bin/ledger-worker ./cmd/worker
	go build -o bin/ledger-connector ./cmd/connector

api:
	go run ./cmd/api -config configs/config.yaml

gateway:
	go run ./cmd/gateway -config configs/config.yaml

worker:
	go run ./cmd/worker -config configs/config.yaml

connector:
	go run ./cmd/connector -config configs/config.yaml

docker-up:
	docker compose -f deployments/docker-compose.yaml up -d --build

docker-down:
	docker compose -f deployments/docker-compose.yaml down
