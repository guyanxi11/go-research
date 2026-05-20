.PHONY: help tidy run build test fmt vet up down logs psql redis-cli

APP        := go-research
BIN_DIR    := bin
MAIN_PKG   := ./cmd/server

help:
	@echo "Targets:"
	@echo "  make tidy      - go mod tidy"
	@echo "  make run       - run server (reads .env)"
	@echo "  make build     - build binary to $(BIN_DIR)/$(APP)"
	@echo "  make test      - go test ./..."
	@echo "  make fmt       - go fmt ./..."
	@echo "  make vet       - go vet ./..."
	@echo "  make up        - docker compose up -d (postgres + redis)"
	@echo "  make down      - docker compose down"
	@echo "  make logs      - docker compose logs -f"
	@echo "  make psql      - open psql in the postgres container"
	@echo "  make redis-cli - open redis-cli in the redis container"

tidy:
	go mod tidy

run:
	go run $(MAIN_PKG)

build:
	go build -o $(BIN_DIR)/$(APP) $(MAIN_PKG)

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...

up:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f

psql:
	docker compose exec postgres psql -U research -d research

redis-cli:
	docker compose exec redis redis-cli
