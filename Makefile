BINARY=money-wa-bot
MODULE=github.com/ramadiaz/money-wa-bot
DOCKER_IMAGE=money-wa-bot

.PHONY: build run test lint migrate docker-build docker-up docker-down tidy

build:
	go build -trimpath -ldflags="-s -w" -o bin/$(BINARY) ./cmd/$(BINARY)

run:
	go run ./cmd/$(BINARY)

tidy:
	go mod tidy

test:
	go test -race -count=1 ./...

lint:
	golangci-lint run ./...

migrate:
	@if [ -z "$(DATABASE_URL)" ]; then \
		export $$(grep -v '^#' ../.env | xargs) 2>/dev/null; \
	fi; \
	goose -dir migrations postgres "$$DATABASE_URL" up

migrate-down:
	@if [ -z "$(DATABASE_URL)" ]; then \
		export $$(grep -v '^#' ../.env | xargs) 2>/dev/null; \
	fi; \
	goose -dir migrations postgres "$$DATABASE_URL" down

docker-build:
	docker build -t $(DOCKER_IMAGE):latest -f deploy/Dockerfile .

docker-up:
	docker compose -f deploy/docker-compose.yml up -d

docker-down:
	docker compose -f deploy/docker-compose.yml down
