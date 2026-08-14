.PHONY: tidy build api gateway worker docker-up docker-down

tidy:
	go mod tidy

build:
	mkdir -p bin
	go build -o bin/ledger-api ./cmd/api
	go build -o bin/ledger-gateway ./cmd/gateway
	go build -o bin/ledger-worker ./cmd/worker

api:
	go run ./cmd/api -config configs/config.yaml

gateway:
	go run ./cmd/gateway -config configs/config.yaml

worker:
	go run ./cmd/worker -config configs/config.yaml

docker-up:
	docker compose -f deployments/docker-compose.yaml up -d

docker-down:
	docker compose -f deployments/docker-compose.yaml down
