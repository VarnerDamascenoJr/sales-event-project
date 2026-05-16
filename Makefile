.PHONY: up down logs fmt tidy test test-cover build

up:
	docker compose up --build

down:
	docker compose down

logs:
	docker compose logs -f api worker

fmt:
	docker run --rm -v "$$(pwd)":/app -w /app golang:1.22-alpine gofmt -w ./cmd ./internal

tidy:
	docker run --rm -v "$$(pwd)":/app -w /app golang:1.22-alpine go mod tidy

test:
	docker run --rm -v "$$(pwd)":/app -w /app golang:1.22-alpine go test ./...

test-cover:
	docker run --rm -v "$$(pwd)":/app -w /app golang:1.22-alpine go test ./... -coverprofile=coverage.out

build:
	docker compose build api worker
