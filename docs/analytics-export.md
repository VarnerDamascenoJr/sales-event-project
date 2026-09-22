# Export Analitico de Eventos

O comando `analytics-export` transforma o historico operacional do PostgreSQL em
um dataset estatistico versionado. Diferente do `optiflow-export`, que gera um
cenario de planejamento para o `OptiFlow`, este export preserva eventos
individuais e agregados por janela para analise estatistica.

## Executar

```bash
go run ./cmd/analytics-export \
  -sales-event-id 11111111-1111-1111-1111-111111111111 \
  -output /tmp/sales-analytics-export.json
```

Filtros temporais opcionais usam RFC3339:

```bash
go run ./cmd/analytics-export \
  -start 2026-09-01T10:00:00Z \
  -end 2026-09-02T00:00:00Z
```

O comando usa `DATABASE_URL`; quando a variavel nao existe, usa o mesmo padrao
local dos demais entrypoints:

```text
postgres://sales:sales@localhost:5432/sales_event?sslmode=disable
```

## Contrato

O documento gerado usa `schemaVersion: sales-analytics-export.v1` e inclui:

- eventos individuais com timestamps normalizados em UTC;
- agregados por janelas de `1m`, `5m`, `1h` e `1d`;
- contagens por tipo de evento;
- contagens por status de venda, pagamento, outbox e email;
- contagem de check-ins;
- quantidade de tickets por tipo quando a dimensao existe;
- campos de rastreabilidade: `requestId`, `correlationId` e `transactionId`.

## Eventos Exportados

| Evento | Origem | Observacao |
| --- | --- | --- |
| `sale.created` | `sales.created_at` | Representa venda persistida pelo worker |
| `sale.item.created` | `sale_items.created_at` | Preserva demanda por tipo de ticket |
| `payment.processed` | `payments.processed_at` | Inclui status, provider e valor |
| `outbox.created` | `outbox_events.created_at` | Inclui tipo de evento, status e attempts |
| `outbox.published` | `outbox_events.published_at` | Exportado apenas quando publicado |
| `email.status` | `email_notifications.created_at` | Estado operacional atual do envio |
| `email.sent` | `email_notifications.sent_at` | Exportado apenas quando existe timestamp |
| `email.delivered` | `email_notifications.delivered_at` | Exportado apenas quando existe timestamp |
| `email.opened` | `email_notifications.opened_at` | Exportado apenas quando existe timestamp |
| `email.clicked` | `email_notifications.clicked_at` | Exportado apenas quando existe timestamp |
| `email.bounced` | `email_notifications.bounced_at` | Exportado apenas quando existe timestamp |
| `checkin.completed` | `ticket_check_ins.checked_in_at` | Conta demanda realizada no evento |

## Fixture

Uma fixture pequena, com duas janelas de 5 minutos, esta em:

- `tests/fixtures/sales-analytics-export.v1.json`

Ela e sintetica e serve para validar o contrato do export, nao para representar
uma conclusao operacional real.

## Limitacoes

- Alguns eventos sao derivados de tabelas que guardam o estado atual da entidade.
  Por exemplo, `sale.created` usa `sales.created_at`, mas o status da venda e o
  status observado no momento do export.
- Eventos de email combinam o estado operacional atual com timestamps de eventos
  especificos quando eles existem. Analises de funil devem declarar essa regra
  antes de interpretar conversoes.
- Janelas sem eventos nao sao materializadas no JSON; consumidores devem criar
  janelas vazias quando precisarem de series temporais densas.
