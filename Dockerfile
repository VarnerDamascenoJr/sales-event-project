FROM golang:1.22-alpine AS build

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/worker ./cmd/worker
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/migrate ./cmd/migrate

FROM alpine:3.20 AS api

RUN adduser -D -H appuser
WORKDIR /app
COPY --from=build /out/api /app/api
USER appuser
EXPOSE 8080
ENTRYPOINT ["/app/api"]

FROM alpine:3.20 AS worker

RUN adduser -D -H appuser
WORKDIR /app
COPY --from=build /out/worker /app/worker
USER appuser
EXPOSE 9091
ENTRYPOINT ["/app/worker"]

FROM alpine:3.20 AS migrate

RUN adduser -D -H appuser
WORKDIR /app
COPY --from=build /out/migrate /app/migrate
COPY migrations /app/migrations
USER appuser
ENTRYPOINT ["/app/migrate"]
