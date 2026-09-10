# Architecture

## Visão geral

- `Fato confirmado`: o repositório implementa um backend de venda de ingressos para eventos em Go `1.22` (`go.mod`).
- `Fato confirmado`: o framework HTTP principal é `gin-gonic/gin` (`go.mod`, `internal/httpapi/router.go`).
- `Fato confirmado`: persistência relacional usa PostgreSQL acessado diretamente com `pgx/v5`, sem ORM (`internal/database/postgres.go`, `internal/httpapi/sales_store.go`, `internal/worker/*.go`).
- `Fato confirmado`: mensageria usa RabbitMQ com exchange do tipo `topic` (`internal/messaging/rabbitmq.go`).
- `Fato confirmado`: observabilidade usa Prometheus, Loki, Promtail e Grafana (`docker-compose.yml`, `deployments/*`).

## Estilo arquitetural predominante

- `Fato confirmado`: a arquitetura é modular por pacote, com composição manual de dependências nos entrypoints.
- `Fato confirmado`: há separação clara entre:
  - transporte HTTP em `internal/httpapi`;
  - acesso a banco em stores e processadores;
  - mensageria em `internal/messaging`;
  - processamento assíncrono em `internal/worker`;
  - publicação confiável em `internal/outbox`;
  - aspectos transversais em `internal/config`, `internal/metrics` e `internal/observability`.
- `Inferência`: o estilo se aproxima de uma arquitetura em camadas com componentes orientados a eventos, mais do que Clean Architecture formal. Não há casos de uso ou portas/adapters explicitamente modelados com interfaces em toda a aplicação.

## Entry points

- `cmd/api/main.go`
  - carrega configuração;
  - abre PostgreSQL;
  - conecta RabbitMQ;
  - instancia rate limiter opcional;
  - monta o roteador HTTP;
  - inicia servidor HTTP.
- `cmd/worker/main.go`
  - carrega configuração;
  - abre PostgreSQL;
  - abre duas conexões RabbitMQ;
  - consome filas `sale.created` e `sale.completed`;
  - sobe servidor de métricas do worker;
  - inicia goroutines de outbox e retenção.
- `cmd/email-retry-worker/main.go`
  - carrega configuração;
  - abre PostgreSQL;
  - sobe servidor de métricas próprio;
  - executa o retry de emails `FAILED` em ciclo separado do consumo RabbitMQ.
- `cmd/migrate/main.go`
  - carrega configuração;
  - abre PostgreSQL;
  - aplica migrations SQL a partir de `migrations/`.

## Composição de dependências

- `Fato confirmado`: não existe framework de injeção de dependência.
- `Fato confirmado`: as dependências são montadas manualmente no `main` e passadas por construtores simples:
  - `httpapi.NewRouter(...)`
  - `worker.NewSalesProcessor(db)`
  - `worker.NewTicketDeliveryProcessor(db, sender)`
  - `outbox.NewPublisher(db, broker)`
  - `retention.NewCleaner(db)`

## Fluxo entre camadas

### API

1. `router.go` recebe a requisição.
2. Middlewares aplicam recovery, métricas, autenticação e limitações.
3. Handlers validam o payload.
4. Handlers chamam `SalesStore`/`AuthStore` ou publicam evento em RabbitMQ.
5. O store executa SQL direto e retorna DTOs.

### Worker

1. RabbitMQ entrega mensagem.
2. Processador faz unmarshal do evento.
3. Processador abre transação no PostgreSQL.
4. Persistência e efeitos secundários são aplicados.
5. A mensagem é `Ack` ou `Nack(false, false)`.

## Componentes síncronos

- `GET /healthz`
- `GET /metrics`
- `POST /sales`
- `POST /sales/:saleId/payment-intents`
- `POST /sales/:saleId/payments`
- `POST /webhooks/payments`
- `POST /webhooks/email-events`
- `GET /sales-events/:salesEventId/sales`
- `GET /sales-events/:salesEventId/sales/:saleId`
- `POST /sales-events/:salesEventId/check-ins`

## Componentes assíncronos

- consumo de `sale.created` (`internal/worker/sales_processor.go`);
- consumo de `sale.completed` (`internal/worker/ticket_delivery_processor.go`);
- publicação da outbox (`internal/outbox/publisher.go`);
- retry de emails (`cmd/email-retry-worker`, `internal/worker/ticket_delivery_processor.go`);
- limpeza de retenção (`internal/retention/cleaner.go`).

## Limites e acoplamentos importantes

- `Fato confirmado`: `internal/httpapi/sales_store.go` concentra a maior parte das regras transacionais de pagamento, check-in e eventos de email.
- `Fato confirmado`: `internal/worker/sales_processor.go` depende do shape do evento publicado pela API e do schema relacional.
- `Fato confirmado`: `internal/worker/ticket_delivery_processor.go` depende do status da venda, do histórico relacional e do sucesso do email para marcar estado.
- `Fato confirmado`: `internal/outbox/publisher.go` depende de `event_type` textual, não de enum forte.

## Observabilidade e runtime

- `Fato confirmado`: logs são JSON via `slog` (`internal/observability/logging.go`).
- `Fato confirmado`: `/metrics` da API usa `promhttp.Handler()`.
- `Fato confirmado`: o worker sobe um segundo servidor HTTP só para `/healthz` e `/metrics`.
- `Fato confirmado`: Loki/Promtail/Grafana só aparecem na stack Docker local; não há deploy descrito para produção.

## Infraestrutura identificada

- `Fato confirmado`: Docker multi-stage build no `Dockerfile`.
- `Fato confirmado`: `docker-compose.yml` sobe `postgres`, `rabbitmq`, `migrate`, `api`, `worker`, `email-retry-worker`, `prometheus`, `loki`, `promtail` e `grafana`.
- `Desconhecido`: ambiente de produção real, provedor cloud e estratégia de deploy fora do Compose.

## Riscos arquiteturais

- `Fato confirmado`: `SALE_FAILED` é publicado pela outbox, mas não há fila ligada a `sale.failed` declarada no bootstrap atual.
- `Fato confirmado`: o worker dá `Nack(false, false)` quando falha processando mensagem; como as filas têm DLX, a mensagem pode ir para `sales.dlq`, mas isso não é observado ou consumido na aplicação.
- `Fato confirmado`: o fluxo de criação de venda é eventual; a API aceita antes de garantir reserva.
- `Fato confirmado`: não há abstração de provider de pagamento além da criação de `payment_intents`; o provider é representado por strings e webhooks.
- `Fato confirmado`: o publish RabbitMQ usa `mandatory=false` e o código não habilita publisher confirms; assim, um `PublishJSON` bem-sucedido não prova que existia fila ligada ao routing key nem que algum consumidor receberá a mensagem.
