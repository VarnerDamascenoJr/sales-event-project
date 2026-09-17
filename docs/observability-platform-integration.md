# Integracao com a plataforma de observabilidade

Este roteiro conecta o `sales-event-project` ao Collector da
`operational-observability-platform` para demonstrar uma venda real com traces,
logs e metricas correlacionaveis.

## Pre-requisitos

- Docker/Compose disponivel.
- A plataforma de observabilidade deve estar de pe primeiro. No repositorio
  `operational-observability-platform`:

```bash
docker compose up -d --wait otel-collector prometheus tempo loki grafana
```

O Compose da plataforma cria a rede compartilhada
`operational-observability-network`. O Compose deste projeto conecta API, worker
e email retry worker nessa rede para acessar `http://otel-collector:4318`.

## Subir o fluxo de venda integrado

No `sales-event-project`, suba somente os servicos de dominio. Evite subir o
Prometheus/Loki/Grafana locais deste repositorio durante a demo integrada para
nao conflitar com as portas da plataforma.

```bash
OTEL_ENABLED=true \
OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318 \
OTEL_SERVICE_NAMESPACE=portfolio \
docker compose up --build postgres rabbitmq migrate api worker email-retry-worker
```

Servicos OTEL esperados:

| Processo | `service.name` |
| --- | --- |
| API | `sales-event-api` |
| Worker RabbitMQ/outbox | `sales-event-worker` |
| Email retry worker | `sales-event-email-retry-worker` |

## Gerar uma venda rastreavel

Use um identificador facil de buscar nos logs:

```bash
curl -i -X POST http://localhost:8080/sales \
  -H 'Content-Type: application/json' \
  -H 'X-Correlation-ID: corr-demo-sale-001' \
  -d '{
    "salesEventId": "11111111-1111-1111-1111-111111111111",
    "customerId": "customer-001",
    "customerName": "Ada Lovelace",
    "customerEmail": "ada@example.com",
    "items": [
      {
        "ticketId": "22222222-2222-2222-2222-222222222222",
        "quantity": 1,
        "unitPrice": 10000
      }
    ]
  }'
```

Guarde o `saleId`, `x-request-id`, `x-correlation-id` e `x-transaction-id`
retornados. A API publica `SALE_CREATED` com `traceparent`,
`x-request-id`, `x-correlation-id` e `x-transaction-id`; o worker extrai esses
headers antes de processar a mensagem.

## Conferir na plataforma

No Grafana da plataforma (`http://localhost:3001`, `admin` / `admin`):

- Tempo: busque traces de `sales-event-api` e `sales-event-worker`; uma venda
  deve mostrar spans HTTP, publish RabbitMQ, consume RabbitMQ e processamento do
  worker no mesmo trace.
- Loki: filtre logs por correlacao:

```logql
{service_name=~"sales-event-api|sales-event-worker"} | json | correlation_id="corr-demo-sale-001"
```

- Prometheus: confira os targets e metricas:

```promql
up{job=~"sales-event-api|sales-event-worker|sales-event-email-retry-worker"}
http_requests_total{service="sales-event-api",environment="development"}
worker_messages_processed_total{service="sales-event-worker",environment="development"}
```

## Cleanup

No `sales-event-project`:

```bash
docker compose down
```

Na plataforma, quando terminar a demonstracao:

```bash
docker compose down
```
