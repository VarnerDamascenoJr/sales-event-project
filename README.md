# Sales Event Project

Backend moderno em Go usando Gin, RabbitMQ, PostgreSQL, Prometheus, Loki e Grafana.

O projeto modela uma venda de ingressos como fluxo orientado a eventos:

```text
Cliente -> Gin API -> RabbitMQ -> Worker Go -> PostgreSQL
                              \-> Email Retry Worker
                              \-> Prometheus -> Grafana
```

## Convencoes de correlacao

Este projeto adota as convencoes compartilhadas do portfolio para
`X-Request-ID`, `X-Correlation-ID`, `X-Transaction-ID`, metadados de eventos,
logs e traces. A fonte de verdade esta no documento
[`portfolio-correlation-conventions.md`](https://github.com/VarnerDamascenoJr/OptiFlow/blob/main/docs/portfolio-correlation-conventions.md).

## Componentes

- `cmd/api`: API HTTP em Gin. Valida a venda, publica `SALE_CREATED`, cria intents de pagamento, recebe webhooks assinados de pagamento e grava eventos na outbox.
- `cmd/worker`: consumidor RabbitMQ e publicador de outbox. Processa `SALE_CREATED`, reserva ingressos, publica eventos pendentes da outbox, consome `SALE_COMPLETED`, emite tickets únicos com QR Code e tenta enviar o email ao comprador.
- `cmd/email-retry-worker`: worker dedicado ao retry de notificacoes de email com status `FAILED`.
- `migrations`: migrations versionadas para schema e seed local.
- `deployments/prometheus`: configuração de scrape da API, do worker e do worker de retry de email.
- `deployments/loki` e `deployments/promtail`: coleta e armazenamento de logs dos containers.
- `deployments/grafana`: datasources e dashboard provisionados.

## Como rodar

```bash
docker compose up --build
```

O serviço `migrate` aplica as migrations antes da API, do worker e do worker de retry de email iniciarem.

Serviços principais:

- API: `http://localhost:8080`
- RabbitMQ Management: `http://localhost:15672` (`guest` / `guest`)
- Prometheus: `http://localhost:9090`
- Email Retry Worker metrics: `http://localhost:9092`
- Loki: `http://localhost:3100`
- Grafana: `http://localhost:3000` (`admin` / `admin`)

## Criar venda

As migrations já criam um evento publicado e dois tickets para teste local.

```bash
curl -X POST http://localhost:8080/sales \
  -H 'Content-Type: application/json' \
  -d '{
    "salesEventId": "11111111-1111-1111-1111-111111111111",
    "customerId": "customer-001",
    "customerName": "Ada Lovelace",
    "customerEmail": "ada@example.com",
    "items": [
      {
        "ticketId": "22222222-2222-2222-2222-222222222222",
        "quantity": 2,
        "unitPrice": 10000
      }
    ]
  }'
```

Resposta esperada:

```json
{
  "saleId": "generated-uuid",
  "status": "PROCESSING"
}
```

Depois disso, o worker consome `SALE_CREATED`, reserva os ingressos e grava a venda como `PENDING_PAYMENT`.

## Criar intencao de pagamento

Use o `saleId` retornado na criação da venda. Em vez de confirmar o pagamento direto, a API agora cria uma intencao de pagamento para o provider.

```bash
curl -X POST http://localhost:8080/sales/generated-sale-uuid/payment-intents \
  -H 'Content-Type: application/json' \
  -d '{
    "amount": 20000,
    "provider": "credit_card"
  }'
```

Resposta esperada:

```json
{
  "id": "generated-payment-intent-uuid",
  "saleId": "generated-sale-uuid",
  "status": "PENDING",
  "provider": "credit_card",
  "amount": 20000,
  "providerReference": "pay_generated-reference",
  "clientSecret": "pi_generated-secret"
}
```

## Confirmar pagamento por webhook

Quando o provider confirma o pagamento, ele envia um webhook assinado. A API valida a assinatura, grava a venda como `COMPLETED` ou `FAILED` e registra `SALE_COMPLETED` ou `SALE_FAILED` na outbox dentro da mesma transação. O worker publica esse evento no RabbitMQ, consome a mensagem, cria `issued_tickets` e envia os QR Codes por email.

```bash
PAYLOAD='{
  "paymentIntentId": "generated-payment-intent-uuid",
  "saleId": "generated-sale-uuid",
  "provider": "credit_card",
  "status": "APPROVED",
  "amount": 20000,
  "occurredAt": "2026-05-30T19:00:00Z",
  "providerReference": "provider-reference-123"
}'

SIGNATURE=$(printf '%s' "$PAYLOAD" | openssl dgst -sha256 -hmac 'change-me-payment-webhook-secret' -binary | xxd -p -c 256)

curl -X POST http://localhost:8080/webhooks/payments \
  -H 'Content-Type: application/json' \
  -H "X-Webhook-Signature: $SIGNATURE" \
  -d "$PAYLOAD"
```

Resposta esperada:

```json
{
  "saleId": "generated-sale-uuid",
  "saleStatus": "COMPLETED",
  "payment": {
    "status": "APPROVED",
    "amount": 20000,
    "provider": "credit_card",
    "processedAt": "2026-05-22T12:00:00Z"
  }
}
```

Para simular falha de pagamento, envie `"status": "FAILED"` no webhook. Nesse caso, a venda vira `FAILED` e a reserva dos ingressos volta para o estoque.

Se `SMTP_HOST` não estiver configurado, o worker apenas registra no log que o envio foi ignorado. Para envio real, configure:

- `SMTP_HOST`
- `SMTP_PORT`
- `SMTP_USERNAME`
- `SMTP_PASSWORD`
- `SMTP_FROM`
- `EMAIL_PROVIDER`
- `EMAIL_WEBHOOK_SECRET`
- `PAYMENT_WEBHOOK_SECRET`
- `METRICS_PROTECTED`
- `PUBLIC_RATE_LIMIT_ENABLED`
- `PUBLIC_RATE_LIMIT_REQUESTS_PER_SECOND`
- `PUBLIC_RATE_LIMIT_BURST`
- `EMAIL_RETRY_ENABLED`
- `EMAIL_RETRY_INTERVAL`
- `EMAIL_RETRY_METRICS_HOST`
- `EMAIL_RETRY_METRICS_PORT`

## Retencao de dados temporarios

O worker tambem executa uma limpeza periodica para reduzir dados operacionais que nao precisam ficar no banco para sempre:

- `outbox_events` publicados ha mais de `RETENTION_PUBLISHED_OUTBOX_MAX_AGE`.
- `email_notifications` com status `SENT` ha mais de `RETENTION_SENT_EMAIL_MAX_AGE`.

Valores padrao:

```env
RETENTION_ENABLED=true
RETENTION_INTERVAL=24h
RETENTION_PUBLISHED_OUTBOX_MAX_AGE=720h
RETENTION_SENT_EMAIL_MAX_AGE=2160h
```

Vendas, pagamentos, tickets emitidos e check-ins nao sao apagados por essa rotina, porque fazem parte do historico comercial e de auditoria do evento.

## Retry de email

Quando o envio do ticket por email falha, o worker principal registra o erro em `email_notifications` e agenda novas tentativas. O reenvio fica em um processo separado, `cmd/email-retry-worker`, que pode ser iniciado, parado e observado sem interromper o consumo principal de vendas e tickets.

- `FAILED`: falhou e entrara em retry depois de `next_retry_at`.
- `DEAD_LETTER`: excedeu o limite de tentativas.
- `SENT`: envio concluido.

Configuracao:

```env
EMAIL_RETRY_ENABLED=true
EMAIL_RETRY_INTERVAL=1m
EMAIL_RETRY_METRICS_HOST=0.0.0.0
EMAIL_RETRY_METRICS_PORT=9092
```

O retry usa backoff exponencial, limitado a 1 hora. Depois de 5 tentativas sem sucesso, a notificacao vira `DEAD_LETTER`.

## Eventos de email

O sistema tambem aceita eventos de provider por webhook para acompanhar o ciclo do email depois do envio:

- `DELIVERED`
- `OPENED`
- `CLICKED`
- `BOUNCED`

Endpoint:

```bash
curl -X POST http://localhost:8080/webhooks/email-events \
  -H 'Content-Type: application/json' \
  -H 'X-Webhook-Secret: change-me-email-webhook-secret' \
  -d '{
    "saleId": "generated-sale-uuid",
    "eventType": "OPENED",
    "occurredAt": "2026-05-30T18:30:00Z",
    "providerEventId": "provider-event-123"
  }'
```

Esses eventos atualizam `email_notifications` para refletir entrega, abertura, clique ou bounce. Retentativa automatica continua sendo usada apenas para falha de envio, nao para falta de abertura.

O ambiente agora fica preparado para provider externo por configuracao:

```env
EMAIL_PROVIDER=generic
EMAIL_WEBHOOK_SECRET=change-me-email-webhook-secret
PAYMENT_WEBHOOK_SECRET=change-me-payment-webhook-secret
METRICS_PROTECTED=false
PUBLIC_RATE_LIMIT_ENABLED=true
PUBLIC_RATE_LIMIT_REQUESTS_PER_SECOND=5
PUBLIC_RATE_LIMIT_BURST=10
WORKER_METRICS_HOST=0.0.0.0
EMAIL_RETRY_METRICS_HOST=0.0.0.0
EMAIL_RETRY_METRICS_PORT=9092
```

Primeira versao:

- `generic`: payload interno simples, bom para desenvolvimento e para adaptar depois.

Proximos adapters naturais:

- `sendgrid`
- `ses`
- `mailgun`
- `postmark`

## Autenticacao local

Alguns endpoints usam API Key no header `X-API-Key`. As migrations criam chaves locais para desenvolvimento:

- `dev-admin-key`: role `ADMIN`.
- `dev-support-key`: role `SUPPORT`.
- `dev-check-in-key`: role `CHECK_IN`.
- `dev-payment-provider-key`: role `PAYMENT_PROVIDER`.

Permissoes iniciais:

- `POST /sales`: publico.
- `POST /sales/:saleId/payment-intents`: publico.
- `POST /sales/:saleId/payments`: `PAYMENT_PROVIDER` ou `ADMIN`.
- `POST /webhooks/payments`: webhook assinado por `PAYMENT_WEBHOOK_SECRET`.
- `GET /sales-events/:salesEventId/sales`: `SUPPORT` ou `ADMIN`.
- `GET /sales-events/:salesEventId/sales/:saleId`: `SUPPORT` ou `ADMIN`.
- `POST /sales-events/:salesEventId/check-ins`: `CHECK_IN` ou `ADMIN`.
- `/healthz`: publico.
- `/metrics`: publico em `development` por padrao e protegido por `ADMIN` nos ambientes mais fechados.
- metricas do worker: use `WORKER_METRICS_HOST=127.0.0.1` fora de desenvolvimento para deixá-las apenas na rede interna da máquina/container.

## Validar entrada

Depois que o ticket é emitido, o QR Code contém um payload no formato `issued_ticket:{issuedTicketId}`. Use esse valor no check-in do evento:

```bash
curl -X POST http://localhost:8080/sales-events/11111111-1111-1111-1111-111111111111/check-ins \
  -H 'Content-Type: application/json' \
  -H 'X-API-Key: dev-check-in-key' \
  -d '{
    "ticketCode": "issued_ticket:generated-issued-ticket-uuid"
  }'
```

Resposta esperada:

```json
{
  "checkInId": "generated-check-in-uuid",
  "issuedTicketId": "generated-issued-ticket-uuid",
  "salesEventId": "11111111-1111-1111-1111-111111111111",
  "saleId": "generated-sale-uuid",
  "ticketId": "22222222-2222-2222-2222-222222222222",
  "ticketName": "General Admission",
  "customerId": "customer-001",
  "customerName": "Ada Lovelace",
  "checkedInAt": "2026-05-22T12:00:00Z"
}
```

O mesmo ticket nao pode entrar duas vezes. A tabela `ticket_check_ins` tem uma restricao unica por `issued_ticket_id`, entao leituras duplicadas do mesmo QR Code retornam conflito.

## Consultar vendas

Listar vendas de um evento:

```bash
curl 'http://localhost:8080/sales-events/11111111-1111-1111-1111-111111111111/sales?page=1&pageSize=20&status=COMPLETED' \
  -H 'X-API-Key: dev-support-key'
```

Filtros disponíveis:

- `page`: página atual. Padrão: `1`.
- `pageSize`: tamanho da página. Padrão: `20`, máximo: `100`.
- `status`: `PENDING_PAYMENT`, `COMPLETED` ou `FAILED`.
- `eventName`: filtro parcial pelo nome do evento.

Buscar uma venda específica dentro de um evento:

```bash
curl 'http://localhost:8080/sales-events/11111111-1111-1111-1111-111111111111/sales/generated-sale-uuid' \
  -H 'X-API-Key: dev-support-key'
```

## Observabilidade

Endpoints:

- API: `http://localhost:8080/metrics`
- Worker: `http://localhost:9091/metrics`
- Email Retry Worker: `http://localhost:9092/metrics`

Métricas principais:

- `http_requests_total`
- `http_request_duration_seconds`
- `sales_created_total`
- `payments_processed_total`
- `events_published_total`
- `outbox_events_processed_total`
- `worker_sales_processed_total`
- `worker_messages_processed_total`
- `worker_message_processing_duration_seconds`
- `ticket_delivery_total`
- `issued_tickets_total`

Logs:

- Os serviços Go escrevem logs estruturados em JSON.
- Campos sensiveis como email/recipient/segredos sao redigidos nos logs.
- Promtail coleta logs dos containers Docker e envia para Loki.
- Grafana tem datasources de Prometheus e Loki provisionados.

Consultas úteis no Grafana Explore:

```logql
{service="api"}
{service="worker"}
{service="email-retry-worker"}
{service="worker"} |= "ticket email delivered"
{service="email-retry-worker"} |= "retry ticket email"
{service="worker"} | json | sale_id="generated-sale-uuid"
{service=~"api|worker|email-retry-worker"} | json | correlation_id="corr-generated-uuid"
{service=~"api|worker|email-retry-worker"} | json | transaction_id="sale-uuid"
```

Consultas úteis em Prometheus:

```promql
sum by (status, provider) (payments_processed_total)
sum by (status) (ticket_delivery_total)
sum by (queue, status) (worker_messages_processed_total)
histogram_quantile(0.95, sum by (le, queue) (rate(worker_message_processing_duration_seconds_bucket[5m])))
```

## Testes

Os testes unitários ficam próximos do pacote testado, seguindo o padrão comum em Go.

```bash
make test
```

## Qualidade de código

Formatar e corrigir problemas simples:

```bash
make lint-fix
```

Rodar lint sem alterar arquivos:

```bash
make lint
```

Instalar o hook local de pre-commit:

```bash
make install-hooks
```

O hook roda `make lint-fix` e `make test`. Se algum arquivo for alterado automaticamente, o commit é interrompido para você revisar e adicionar as mudanças.

Rodar o fluxo de integração com Docker Compose:

```bash
make test-integration
```

Esse teste sobe `postgres`, `rabbitmq`, `migrate`, `api` e `worker`, aplica as migrations versionadas, cria uma venda real, aguarda a reserva assíncrona, cria a intencao de pagamento, confirma o pagamento por webhook assinado, aguarda emissão de tickets/QR Code, valida o check-in e verifica que o mesmo QR Code nao entra duas vezes. Ele também cobre falha de pagamento restaurando estoque.

Aplicar migrations manualmente:

```bash
make migrate
```

Estrutura atual:

- `internal/httpapi/router_test.go`: testa handlers Gin, validações HTTP e publicação de eventos usando fakes.
- `internal/httpapi/sales_store.go`: isola o SQL em um store Postgres, deixando o router testável sem banco real.
- `internal/notification`: monta e envia o email com anexos PNG de QR Code.

## Eventos iniciais

- `SALE_CREATED`: publicado pela API após validação.
- `SALE_COMPLETED`: registrado na outbox e publicado no RabbitMQ quando o pagamento é aprovado.
- `SALE_FAILED`: registrado na outbox quando a reserva ou pagamento falha.

Contrato de contexto RabbitMQ:

- Headers AMQP:
  - `x-request-id`
  - `x-correlation-id`
  - `x-transaction-id`
- Payload:
  - `SALE_CREATED` inclui `metadata.requestId`, `metadata.correlationId` e
    `metadata.transactionId`.
  - `SALE_COMPLETED` inclui o mesmo bloco `metadata` recuperado da venda.
  - `SALE_FAILED` inclui `metadata` no payload persistido na outbox.
- Trace context:
  - quando OpenTelemetry esta ativo, o publisher injeta headers W3C como
    `traceparent`/`baggage`; o worker extrai esses headers antes de processar a
    mensagem.

O worker monta o contexto dos logs combinando headers AMQP e payload. Headers
recebidos tem precedencia; campos ausentes sao preenchidos pelo `metadata` do
payload.

## Outbox com retry

Eventos em `outbox_events` usam estado para publicação confiável:

- `PENDING`: aguardando primeira publicação.
- `FAILED`: falhou e será tentado novamente depois de `next_attempt_at`.
- `PUBLISHED`: publicado com sucesso.
- `DEAD_LETTER`: excedeu o limite de tentativas.

O worker tenta publicar eventos `PENDING` ou `FAILED` com `next_attempt_at <= NOW()`. Em caso de falha, incrementa `attempts`, grava `last_error` e agenda novo retry com backoff exponencial, limitado a 1 hora. Depois de 5 tentativas, o evento vira `DEAD_LETTER`.

Publicacoes RabbitMQ usam publisher confirms e `mandatory=true`. A outbox so marca
um evento como `PUBLISHED` depois que o broker confirma a publicacao e a mensagem
nao e devolvida por falta de rota. Se o broker responder com `nack`, se a mensagem
for retornada como nao roteavel ou se a confirmacao expirar pelo contexto, o evento
permanece em fluxo de retry e pode chegar a `DEAD_LETTER`.

## Próximos passos naturais

- Criar cenário controlado para exercitar o `email-retry-worker` com SMTP falho.
