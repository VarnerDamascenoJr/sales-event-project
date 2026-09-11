# Database

## Tecnologia e acesso

- `Fato confirmado`: o banco usado é PostgreSQL.
- `Fato confirmado`: acesso via `pgxpool`, sem ORM.
- `Fato confirmado`: o pool usa:
  - `MaxConns = 10`
  - `MinConns = 1`
  - `MaxConnLifetime = 1h`
  - `MaxConnIdleTime = 30m`

## Estratégia de migrations

- `Fato confirmado`: as migrations ficam em `migrations/`.
- `Fato confirmado`: existe um runner próprio em `cmd/migrate/main.go`.
- `Fato confirmado`: o runner:
  - lê arquivos `.sql`;
  - extrai a seção `-- +goose Up`;
  - ordena pela versão do nome;
  - cria a tabela `schema_migrations`;
  - aplica cada migration dentro de transação.
- `Fato confirmado`: ele não usa a biblioteca `goose`; só reutiliza o formato dos comentários.

## Limitação do runner

- `Fato confirmado`: `splitSQLStatements` separa por `;` puro.
- `Inferência`: isso pode quebrar migrations futuras com funções SQL, blocos `DO $$`, triggers ou statements complexos com `;` internos.

## Tabelas principais

### `customers`

- PK: `id VARCHAR(50)`
- `email` único
- campos: `email`, `name`, timestamps

### `sales_events`

- PK: `id UUID`
- campos: `name`, `status`, `starts_at`, `created_at`
- status permitido:
  - `DRAFT`
  - `PUBLISHED`
  - `CANCELLED`

### `tickets`

- PK: `id UUID`
- FK: `sales_event_id -> sales_events(id)`
- campos: `name`, `price`, `available_quantity`, `created_at`

### `sales`

- PK: `id UUID`
- FK: `sales_event_id -> sales_events(id)`
- FK: `customer_id -> customers(id)`
- campos: `status`, `total_amount`, `request_id`, `correlation_id`,
  `transaction_id`, `created_at`, `updated_at`

### `sale_items`

- PK: `id UUID`
- FK: `sale_id -> sales(id)`
- FK: `ticket_id -> tickets(id)`
- campos: `quantity`, `unit_price`, `created_at`
- unicidade: `(sale_id, ticket_id)`

### `payments`

- PK: `id UUID`
- FK única: `sale_id -> sales(id)`
- campos: `status`, `amount`, `provider`, `processed_at`

### `payment_intents`

- PK: `id UUID`
- FK única: `sale_id -> sales(id)`
- campos:
  - `provider`
  - `amount`
  - `status`
  - `provider_reference`
  - `client_secret`
  - `created_at`
  - `updated_at`

### `issued_tickets`

- PK: `id UUID`
- FKs:
  - `sale_id -> sales(id)`
  - `ticket_id -> tickets(id)`
  - `customer_id -> customers(id)`
- campos:
  - `sequence`
  - `qr_code_payload` único
  - `emailed_at`
  - `created_at`
- unicidade: `(sale_id, ticket_id, sequence)`

### `email_notifications`

- PK: `id UUID`
- FK única: `sale_id -> sales(id)`
- campos operacionais:
  - `recipient_email`
  - `status`
  - `attempts`
  - `next_retry_at`
  - `error_message`
  - `sent_at`
  - `delivered_at`
  - `opened_at`
  - `clicked_at`
  - `bounced_at`
  - `provider_event_id`
  - `created_at`
  - `updated_at`

### `outbox_events`

- PK: `event_id UUID`
- campos:
  - `event_type`
  - `aggregate_id`
  - `payload JSONB`
  - `status`
  - `attempts`
  - `last_error`
  - `next_attempt_at`
  - `trace_context`
  - `request_id`
  - `correlation_id`
  - `transaction_id`
  - `published_at`
  - `created_at`

### `ticket_check_ins`

- PK: `id UUID`
- FK: `issued_ticket_id -> issued_tickets(id)`
- FK: `sales_event_id -> sales_events(id)`
- unicidade: `issued_ticket_id`

### `api_keys`

- PK: `id UUID`
- campos:
  - `name`
  - `key_hash` único
  - `role`
  - `active`
  - `created_at`
  - `revoked_at`

## Índices visíveis

- `idx_sales_status`
- `idx_tickets_sales_event_id`
- `idx_issued_tickets_sale_id`
- `idx_email_notifications_status`
- `idx_outbox_events_published_at`
- `idx_ticket_check_ins_sales_event_id`
- `idx_api_keys_active_role`
- `idx_outbox_events_status_next_attempt_at`
- `idx_email_notifications_status_next_retry_at`
- `idx_email_notifications_provider_event_id` parcial
- `idx_payment_intents_status`
- `idx_sales_correlation_id` parcial
- `idx_outbox_events_correlation_id` parcial

## Regras de duplicidade e idempotência

- `Fato confirmado`: uma venda tem no máximo um pagamento.
- `Fato confirmado`: uma venda tem no máximo uma payment intent.
- `Fato confirmado`: uma venda tem no máximo uma notificação de email.
- `Fato confirmado`: um ticket emitido é único por `(sale_id, ticket_id, sequence)`.
- `Fato confirmado`: um ticket só pode ter um check-in.
- `Fato confirmado`: `provider_event_id` de email é único quando presente.

## Transações observadas

- `ProcessPayment`
- `CreatePaymentIntent`
- `ProcessPaymentWebhook`
- `CheckInTicket`
- `SalesProcessor.persistSale`
- `TicketDeliveryProcessor.prepareTicketEmail`
- `TicketDeliveryProcessor.RetryFailedEmails`
- `outbox.Publisher.PublishPending`
- `cmd/migrate.applyMigration`

## Locks explícitos

- `Fato confirmado`: `FOR UPDATE` em venda durante pagamento.
- `Fato confirmado`: `FOR UPDATE` em `payment_intents` durante webhook.
- `Fato confirmado`: `FOR UPDATE` em venda no preparo do email.
- `Fato confirmado`: `FOR UPDATE OF it` em ticket emitido no check-in.
- `Fato confirmado`: `FOR UPDATE SKIP LOCKED` em outbox e retry de email.

## Datas e timezone

- `Fato confirmado`: colunas temporais relevantes usam `TIMESTAMPTZ`.
- `Fato confirmado`: a aplicação normaliza diversos horários com `time.Now().UTC()` ou `OccurredAt.UTC()`.
- `Inferência`: a intenção é manter os registros operacionais em UTC.

## Seed local

- `Fato confirmado`: a migration `00002` cria:
  - um `sales_event` publicado;
  - dois tickets (`General Admission` e `VIP`).
- `Fato confirmado`: a migration `00003` cria API keys locais por hash.

## Consistência e riscos

- `Fato confirmado`: a validação de estoque na API e a reserva no worker acontecem em momentos diferentes.
- `Fato confirmado`: falha de reserva no worker impede o commit transacional da venda.
- `Fato confirmado`: `GetSale` faz `JOIN payments`; uma venda sem linha em `payments` não aparece nesse endpoint.
- `Inferência`: como `customers.email` é único, reaproveitar o mesmo email com `customer_id` diferente pode gerar conflito em cenários não cobertos explicitamente.
- `Fato confirmado`: não há cache identificado.
