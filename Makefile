.PHONY: up down logs fmt tidy lint lint-fix install-hooks test migrate test-integration test-cover build

up:
	docker compose up --build

down:
	docker compose down

logs:
	docker compose logs -f api worker email-retry-worker

fmt:
	docker run --rm -v "$$(pwd)":/app -w /app golang:1.22-alpine gofmt -w ./cmd ./internal ./tests

tidy:
	docker run --rm -v "$$(pwd)":/app -w /app golang:1.22-alpine go mod tidy

lint:
	docker run --rm -v "$$(pwd)":/app -w /app golangci/golangci-lint:v1.62.2 golangci-lint run

lint-fix:
	docker run --rm -v "$$(pwd)":/app -w /app golang:1.22-alpine gofmt -w ./cmd ./internal ./tests
	docker run --rm -v "$$(pwd)":/app -w /app golangci/golangci-lint:v1.62.2 golangci-lint run --fix

install-hooks:
	cp scripts/git-hooks/pre-commit .git/hooks/pre-commit
	chmod +x .git/hooks/pre-commit

test:
	docker run --rm -v "$$(pwd)":/app -w /app golang:1.22-alpine go test ./...

migrate:
	docker compose up -d postgres
	docker compose run --rm migrate

test-integration:
	docker compose up --build -d postgres rabbitmq migrate api worker email-retry-worker
	docker run --rm --network host -v "$$(pwd)":/app -w /app golang:1.22-alpine go test -tags=integration ./tests/integration -count=1 -v

test-cover:
	docker run --rm -v "$$(pwd)":/app -w /app golang:1.22-alpine go test ./... -coverprofile=coverage.out

build:
	docker compose build api worker email-retry-worker
