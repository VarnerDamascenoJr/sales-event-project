# Troubleshooting

## Execução local

### Subir ambiente

- comando: `docker compose up --build`

### Ver logs principais

- comando: `docker compose logs -f api worker`

### Rodar migrations manualmente

- comando: `make migrate`
- cuidado: este comando altera o banco local.

## Troubleshooting por sintoma

### `POST /sales` retorna `202`, mas a venda não aparece depois

Possíveis causas confirmadas:

- falha ao processar `SALE_CREATED` no worker;
- falta de estoque no momento real da reserva;
- mensagem encaminhada para DLQ após `Nack(false, false)`.

Onde olhar:

- logs do `worker`;
- tabela `sales`;
- RabbitMQ / `sales.dlq`.

### Pagamento por webhook não conclui a venda

Possíveis causas confirmadas:

- assinatura HMAC inválida;
- `paymentIntentId` não pertence à venda;
- `provider` ou `amount` divergentes da `payment_intent`;
- venda fora de `PENDING_PAYMENT`.

Onde olhar:

- logs da API;
- `payment_intents`;
- `payments`;
- `sales`;
- `outbox_events`.

### Venda falhou, mas não há efeito assíncrono observável

- `Fato confirmado`: `SALE_FAILED` é gravado na outbox, porém o bootstrap atual não declara fila para `sale.failed`.
- efeito prático:
  - o evento não é marcado como `PUBLISHED` sem rota confirmada;
  - o RabbitMQ devolve a mensagem, a outbox grava `last_error` e agenda retry;
  - após o limite de tentativas, o evento vira `DEAD_LETTER`.

### Ticket não foi enviado por email

Possíveis causas:

- `SMTP_HOST` vazio, então o sistema usa `LogSender`;
- falha SMTP colocou a notificação em `FAILED`;
- retries levaram o registro a `DEAD_LETTER`.

Onde olhar:

- logs do `worker`;
- `email_notifications`;
- `issued_tickets`.

### Mesmo QR Code não entra no evento

- `Fato confirmado`: duplicidade de check-in gera conflito por restrição única em `ticket_check_ins`.
- comportamento esperado:
  - primeiro check-in `201`;
  - repetição `409`.

### `/metrics` responde `401` ou `403`

- `Fato confirmado`: quando `METRICS_PROTECTED=true`, a API exige API key com role `ADMIN`.

### Testes com `go test` falham no host

- `Fato confirmado`: o ambiente pode não ter `go` instalado.
- caminho suportado pelo projeto:
  - `make test`
  - `make lint`
  - `docker compose build api worker migrate`

## Comandos seguros de diagnóstico

- `docker compose ps`
- `docker compose logs -f api worker`
- `make test`
- `make lint`
- `docker compose build api worker migrate`

## Convenções e operação

- `Fato confirmado`: existe hook de pre-commit em `scripts/git-hooks/pre-commit`.
- `Fato confirmado`: o hook roda:
  - `make lint-fix`
  - `make test`
- `Desconhecido`: convenções de branches e mensagens de commit.

## Ambientes identificados

- `Fato confirmado`: `development` é explicitamente suportado por defaults.
- `Inferência`: outros ambientes podem existir por `APP_ENV`, mas não estão descritos no repositório.

## Pontos de atenção para mudanças futuras

- Qualquer ajuste em pagamento deve considerar:
  - transação no store;
  - outbox;
  - emissão de ticket;
  - restauração de estoque em falha.
- Qualquer ajuste em filas deve considerar:
  - exchange;
  - DLX;
  - bindings;
  - métricas;
  - consumidores existentes.
- Qualquer ajuste em retenção deve considerar:
  - `outbox_events` publicados;
  - `email_notifications` com `SENT`;
  - possíveis impactos em auditoria.
