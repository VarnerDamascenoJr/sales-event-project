# Business Journey Dashboard

The `Sales Business Journey` dashboard is provisioned in Grafana from
`deployments/grafana/dashboards/sales-business-journey.json`. It is designed to
explain a sale from API acceptance to payment, outbox publication, ticket email,
and check-in without reading the application code.

## Start the local stack

```bash
docker network create operational-observability-network 2>/dev/null || true
docker compose up --build -d postgres rabbitmq migrate api worker email-retry-worker prometheus grafana
```

Open Grafana at `http://localhost:3000` with `admin` / `admin`, then open:

```text
http://localhost:3000/d/sales-business-journey/sales-business-journey
```

Grafana loads the dashboard from the `Sales Event` provisioned folder. Prometheus
is the default datasource and scrapes the API, worker, and email retry worker.

For the cleanest counter deltas in very short demos, open the dashboard and wait
for one Prometheus scrape before running the failure scenarios. The dashboard also
shows current counter values after the services have emitted the metrics.

## Filters

Use the Grafana time picker for the analysis window. The dashboard also exposes
variables for the event and status dimensions used by the business flow:

- sale status from `worker_sales_processed_total`;
- payment status and provider from `payments_processed_total`;
- outbox event type and status from `outbox_events_processed_total`;
- RabbitMQ routing key and publish status from `events_published_total`;
- worker queue and ack/nack status from `worker_messages_processed_total`;
- ticket email status from `ticket_delivery_total`;
- HTTP route and status from `http_requests_total`.

`sales_event_id`, `sale_id`, and customer fields are intentionally not Prometheus
labels to avoid high-cardinality metrics. Use the API, logs, database, or traces
for a single sale drilldown after the dashboard points to the failing stage.

## Panels

| Dashboard area | What it explains | Main metrics |
| --- | --- | --- |
| Journey summary | Sales accepted by the API, sales persisted as `PENDING_PAYMENT`, `COMPLETED`, or `FAILED`, and payment outcomes. | `sales_created_total`, `worker_sales_processed_total`, `payments_processed_total`, `payment_webhook_replays_total` |
| Async pipeline and outbox | Whether the outbox is publishing, retrying, or reaching `DEAD_LETTER`; RabbitMQ publish success/failure; worker throughput and latency. | `outbox_events_processed_total`, `events_published_total`, `worker_messages_processed_total`, `worker_message_processing_duration_seconds` |
| Tickets, emails, and check-ins | Tickets issued, email delivery/retry/dead-letter behavior, successful check-ins, and check-in HTTP errors. | `issued_tickets_total`, `ticket_delivery_total`, `check_ins_created_total`, `http_requests_total` |
| Symptoms and investigation | API errors and latency by route when the business journey stalls or degrades. | `http_requests_total`, `http_request_duration_seconds` |

`outbox_events_processed_total` increments once per processing attempt, so the
outbox panel acts as the attempts-by-event/status view for retry demonstrations.

## Smoke demonstration

Use the controlled failure runner to generate one approved sale, one idempotent
payment replay, and at least one visible failure.

```bash
scripts/run-failure-scenarios.sh duplicate-payment
scripts/run-failure-scenarios.sh outbox-retry
```

Optional email failure evidence:

```bash
scripts/run-failure-scenarios.sh email-failure
```

Expected observations in the dashboard after setting the time window to the last
30 minutes:

- `Vendas aceitas pela API` increases for each created sale.
- `Vendas pendentes, concluídas e falhas` shows `PENDING_PAYMENT`, `COMPLETED`,
  and `FAILED` when the approved and failed payment scenarios run.
- `Pagamentos aprovados e recusados` shows `APPROVED` and `FAILED` for provider
  `failure_scenario`.
- `Pagamentos duplicados idempotentes` increases after `duplicate-payment` sends
  the same signed webhook twice.
- `Outbox por evento, estado e tentativas` shows `SALE_COMPLETED` published and
  `SALE_FAILED` moving through retry/dead-letter states.
- `Publicações RabbitMQ por rota e status` shows successful publications and the
  failed `sale.failed` routing attempt used by the outbox retry scenario.
- `Tickets emitidos` and `Entrega de emails` show ticket generation and delivery
  attempts after an approved sale.
- `Tentativas de check-in por status HTTP` is populated when the check-in endpoint
  is exercised by a demo or integration test.

Useful spot checks:

```bash
curl -s http://localhost:8080/metrics | grep -E 'sales_created_total|payments_processed_total|payment_webhook_replays_total|check_ins_created_total'
curl -s http://localhost:9091/metrics | grep -E 'outbox_events_processed_total|events_published_total|worker_messages_processed_total|issued_tickets_total|ticket_delivery_total'
```

Stop the environment when the review is finished:

```bash
docker compose down
```
