# Domain

## Propósito do sistema

- `Fato confirmado`: o sistema modela a venda de ingressos para eventos com reserva de estoque, pagamento, emissão de tickets com QR Code, envio por email e check-in (`README.md`, `internal/httpapi/router.go`, `internal/worker/*.go`).

## Entidades observáveis

| Entidade | Papel | Evidência |
| --- | --- | --- |
| `sales_events` | evento comercial com data e status | `migrations/00001_create_initial_schema.sql` |
| `tickets` | tipos de ingresso e estoque disponível | `migrations/00001_create_initial_schema.sql` |
| `customers` | comprador identificado por `id`, `email`, `name` | `migrations/00001_create_initial_schema.sql` |
| `sales` | venda principal com status e valor total | `migrations/00001_create_initial_schema.sql` |
| `sale_items` | itens comprados na venda | `migrations/00001_create_initial_schema.sql` |
| `payments` | estado atual do pagamento da venda | `migrations/00001_create_initial_schema.sql` |
| `payment_intents` | intenção de pagamento por sale | `migrations/00007_create_payment_intents.sql` |
| `issued_tickets` | ingressos emitidos por item/sequence com QR payload | `migrations/00001_create_initial_schema.sql` |
| `email_notifications` | estado operacional do envio do ticket | `migrations/00001_create_initial_schema.sql`, `00005`, `00006` |
| `ticket_check_ins` | registro de entrada no evento | `migrations/00001_create_initial_schema.sql` |
| `outbox_events` | eventos pendentes/publicados para mensageria | `migrations/00001_create_initial_schema.sql`, `00004` |
| `api_keys` | autenticação de operadores e provider local | `migrations/00003_create_api_keys.sql` |

## Statuses observáveis

### Venda

- `PENDING_PAYMENT`
- `COMPLETED`
- `FAILED`
- `PROCESSING`

`PROCESSING` aparece no retorno da API e no payload de entrada aceito, mas não é persistido em `sales`; a persistência usa `PENDING_PAYMENT`, `COMPLETED` ou `FAILED`.

### Pagamento

- `PENDING`
- `APPROVED`
- `FAILED`

### Payment intent

- `PENDING`
- `SUCCEEDED`
- `FAILED`

### Email notification

- `PENDING`
- `SENT`
- `DELIVERED`
- `OPENED`
- `CLICKED`
- `BOUNCED`
- `FAILED`
- `DEAD_LETTER`

## Regras de negócio confirmadas

### Criação de venda

- `Fato confirmado`: `salesEventId`, `customerId`, `customerName`, `customerEmail` e `items` são obrigatórios (`internal/httpapi/router.go`).
- `Fato confirmado`: o email do comprador deve ter formato válido.
- `Fato confirmado`: a lista de itens deve ter pelo menos um item.
- `Fato confirmado`: cada item deve ter `ticketId`, `quantity > 0` e `unitPrice > 0`.
- `Fato confirmado`: `quantity` e `unitPrice` têm limite superior técnico definido em `internal/events/events.go`.
- `Fato confirmado`: a soma total não pode exceder `1_000_000_000` centavos.
- `Fato confirmado`: o ingresso informado precisa pertencer ao `salesEventId`.
- `Fato confirmado`: `unitPrice` do request deve bater exatamente com o preço do ticket no banco.
- `Fato confirmado`: a quantidade pedida não pode exceder o `available_quantity` consultado no momento da validação.

### Reserva e persistência da venda

- `Fato confirmado`: a venda só é efetivamente persistida pelo worker após consumir `SALE_CREATED`.
- `Fato confirmado`: o estoque é decrementado no banco com `UPDATE tickets ... WHERE available_quantity >= $1`.
- `Fato confirmado`: se não houver estoque suficiente no momento do consumo, o processador falha.
- `Inferência`: como a validação da API e a reserva real são separadas por mensageria, duas vendas concorrentes podem ser aceitas pela API e competir no worker pelo mesmo estoque.

### Pagamento

- `Fato confirmado`: só é possível criar `payment_intent` para vendas `PENDING_PAYMENT`.
- `Fato confirmado`: o valor da `payment_intent` deve ser idêntico ao total da venda.
- `Fato confirmado`: existe no máximo uma `payment_intent` por venda; a operação faz upsert por `sale_id`.
- `Fato confirmado`: um pagamento aprovado marca a venda como `COMPLETED`.
- `Fato confirmado`: um pagamento falho marca a venda como `FAILED` e devolve o estoque reservado.
- `Fato confirmado`: pagamentos webhook validam coerência entre `paymentIntentId`, `saleId`, `provider` e `amount`.
- `Fato confirmado`: pagamento manual por API key (`POST /sales/:saleId/payments`) continua disponível além do fluxo por webhook.

### Emissão de ticket

- `Fato confirmado`: tickets só são emitidos para venda `COMPLETED`.
- `Fato confirmado`: o ID do ticket emitido é determinístico por `saleID + ticketID + sequence`.
- `Fato confirmado`: o QR payload usa o prefixo `issued_ticket:`.
- `Fato confirmado`: reprocessar `SALE_COMPLETED` não duplica tickets por causa do `ON CONFLICT (sale_id, ticket_id, sequence) DO NOTHING`.

### Envio e retry de email

- `Fato confirmado`: se `SMTP_HOST` não estiver configurado, o envio cai em `LogSender` e apenas registra log.
- `Fato confirmado`: falhas de envio movem a notificação para `FAILED` e agendam retry com backoff exponencial limitado a 1 hora.
- `Fato confirmado`: após 5 tentativas, o status vira `DEAD_LETTER`.
- `Fato confirmado`: retries só consultam notificações `FAILED`.

### Eventos de email

- `Fato confirmado`: o webhook aceita `DELIVERED`, `OPENED`, `CLICKED` e `BOUNCED`.
- `Fato confirmado`: esses eventos atualizam timestamps específicos em `email_notifications`.
- `Fato confirmado`: `provider_event_id` é único quando não nulo.

### Check-in

- `Fato confirmado`: o QR code pode ser enviado como payload completo `issued_ticket:<uuid>` ou apenas o UUID.
- `Fato confirmado`: o check-in só é permitido para ticket emitido cuja venda esteja `COMPLETED` e pagamento `APPROVED`.
- `Fato confirmado`: o mesmo ticket não pode entrar duas vezes; a unicidade está em `ticket_check_ins(issued_ticket_id)`.

## Inferências úteis

- `Inferência`: `customers.email` é único para todo o sistema, então o mesmo email não pode estar associado a múltiplos `customer_id` diferentes.
- `Inferência`: o modelo atual trata `customer_id` como identificador externo fornecido pelo cliente chamador, não como UUID gerado internamente.
- `Inferência`: `email_notifications` tem papel mais operacional do que histórico permanente, pois há retenção automática apenas para registros enviados com sucesso.

## Desconhecidos relevantes

- `Desconhecido`: significado de negócios completo dos status de `sales_events` (`DRAFT`, `PUBLISHED`, `CANCELLED`) além do seed/local.
- `Desconhecido`: regras de reembolso, cancelamento de evento, estorno ou troca de ingresso.
- `Desconhecido`: integração real com provedor de pagamento; hoje o projeto modela a intenção e o webhook, mas não há client HTTP de cobrança.
