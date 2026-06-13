# Sales Event Project Agent Guide

Resumo mínimo para futuros agentes.

## Leitura inicial

1. `README.md`
2. `cmd/api/main.go`
3. `cmd/worker/main.go`
4. `internal/httpapi/router.go`
5. `internal/httpapi/sales_store.go`
6. `internal/worker/sales_processor.go`
7. `internal/worker/ticket_delivery_processor.go`
8. `internal/outbox/publisher.go`
9. `migrations/*.sql`
10. `tests/integration/sales_flow_test.go`

## Resumo do sistema

- Backend em Go `1.22` com Gin, PostgreSQL e RabbitMQ.
- Fluxo principal:
  - `POST /sales` valida e publica `SALE_CREATED`;
  - o worker reserva estoque e persiste a venda;
  - pagamento concluído grava evento na outbox;
  - o publisher publica `SALE_COMPLETED`;
  - o worker emite tickets e tenta enviá-los por email.
- Entrypoints:
  - `cmd/api`
  - `cmd/worker`
  - `cmd/migrate`

## Hotspots

- Não assumir consistência imediata após `POST /sales`; a criação é assíncrona.
- Alterações em pagamento afetam pelo menos `sales`, `payments`, `payment_intents`, `outbox_events`, `issued_tickets` e `email_notifications`.
- `SALE_FAILED` é gravado na outbox, mas o bootstrap atual só declara filas para `sale.created` e `sale.completed`.
- `PublishJSON` usa `mandatory=false` e não habilita publisher confirms; um publish pode ser tratado como sucesso sem garantia de rota/consumo.
- O runner de migrations divide SQL por `;`, o que é frágil para migrations futuras mais complexas.

## Comandos úteis

- `docker compose up --build`
- `docker compose logs -f api worker`
- `make test`
- `make lint`
- `make test-cover`
- `make test-integration`

## Documentação detalhada

- `docs/architecture.md`
- `docs/project-structure.md`
- `docs/domain.md`
- `docs/flows.md`
- `docs/database.md`
- `docs/integrations.md`
- `docs/testing.md`
- `docs/troubleshooting.md`
