# Controlled Failure Scenarios

These scenarios make the reliability behavior demonstrable in a local environment.
They are meant for portfolio demos, incident walkthroughs, and quick regression
checks after changes to payments, RabbitMQ, outbox, workers, or email delivery.

Run the full set:

```bash
scripts/run-failure-scenarios.sh
```

Run one scenario:

```bash
scripts/run-failure-scenarios.sh duplicate-payment
scripts/run-failure-scenarios.sh delayed-consumer
scripts/run-failure-scenarios.sh email-failure
scripts/run-failure-scenarios.sh outbox-retry
```

The script starts the Docker Compose services it needs. The Compose file expects
the external network `operational-observability-network`; the script creates it
when it is missing.

## Scenarios

| Scenario | Failure injected | Expected final state | Useful evidence |
| --- | --- | --- | --- |
| `duplicate-payment` | The same signed payment webhook is sent twice. | Both webhook calls return `202`; the second call is an idempotent replay; the sale remains `COMPLETED`; one payment, one `SALE_COMPLETED` outbox event, and one ticket are preserved. | `payments_processed_total`, API logs for `payment processed`, `payments`, `outbox_events`, `issued_tickets`. |
| `delayed-consumer` | The worker is stopped before a sale is created. | The API returns `202`, the sale is not yet persisted while the worker is stopped, and after worker restart the sale reaches `PENDING_PAYMENT`. | `sales` row count before/after restart, RabbitMQ queue counters, worker logs, `worker_messages_processed_total`. |
| `email-failure` | Worker and retry worker are recreated with an unreachable SMTP host. | Ticket delivery marks email as `FAILED`; accelerated retry moves it to `DEAD_LETTER`. | `email_notifications`, `ticket_delivery_total`, email retry worker logs. |
| `outbox-retry` | A failed payment creates `SALE_FAILED`, which has no local binding. | Publisher confirm/return handling keeps the event out of `PUBLISHED`; accelerated retry moves it to `DEAD_LETTER`. | `outbox_events`, `events_published_total`, `outbox_events_processed_total`, worker logs. |

The email and outbox scenarios accelerate retry state by updating local database
rows after the first real failure. This keeps the demo short while still
exercising the same retry/dead-letter code paths.

## After Running

Inspect logs:

```bash
docker compose logs api worker email-retry-worker
```

Inspect metrics:

```bash
curl -s http://localhost:8080/metrics | grep -E 'payments_processed_total|events_published_total'
curl -s http://localhost:9091/metrics | grep -E 'outbox_events_processed_total|worker_messages_processed_total'
curl -s http://localhost:9092/metrics | grep ticket_delivery_total
```

Stop the environment:

```bash
docker compose down
```
