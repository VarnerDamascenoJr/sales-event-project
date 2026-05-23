# Sales Event Project

Backend moderno em Go usando Gin, RabbitMQ, PostgreSQL, Prometheus e Grafana.

O projeto modela uma venda de ingressos como fluxo orientado a eventos:

```text
Cliente -> Gin API -> RabbitMQ -> Worker Go -> PostgreSQL
                              \-> Prometheus -> Grafana
```

## Componentes

- `cmd/api`: API HTTP em Gin. Valida a venda, publica `SALE_CREATED`, confirma pagamentos e publica `SALE_COMPLETED` quando o pagamento é aprovado.
- `cmd/worker`: consumidor RabbitMQ. Processa `SALE_CREATED`, reserva ingressos, grava a venda como `PENDING_PAYMENT`, consome `SALE_COMPLETED`, emite tickets únicos com QR Code e envia o email ao comprador.
- `migrations`: schema inicial e seed de evento/tickets para testes locais.
- `deployments/prometheus`: configuração de scrape da API e do worker.
- `deployments/grafana`: datasource e dashboard provisionados.

## Como rodar

```bash
docker compose up --build
```

Serviços principais:

- API: `http://localhost:8080`
- RabbitMQ Management: `http://localhost:15672` (`guest` / `guest`)
- Prometheus: `http://localhost:9090`
- Grafana: `http://localhost:3000` (`admin` / `admin`)

## Criar venda

O banco já sobe com um evento publicado e dois tickets.

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

## Pagar venda

Use o `saleId` retornado na criação da venda. Quando o pagamento é aprovado, a API grava a venda como `COMPLETED` e publica `SALE_COMPLETED` no RabbitMQ. O worker consome esse evento, cria um registro em `issued_tickets` para cada ingresso comprado e envia os QR Codes por email.

```bash
curl -X POST http://localhost:8080/sales/generated-sale-uuid/payments \
  -H 'Content-Type: application/json' \
  -d '{
    "amount": 20000,
    "provider": "credit_card"
  }'
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

Para simular falha de pagamento, envie `"status": "FAILED"`. Nesse caso, a venda vira `FAILED` e a reserva dos ingressos volta para o estoque.

Se `SMTP_HOST` não estiver configurado, o worker apenas registra no log que o envio foi ignorado. Para envio real, configure:

- `SMTP_HOST`
- `SMTP_PORT`
- `SMTP_USERNAME`
- `SMTP_PASSWORD`
- `SMTP_FROM`

## Consultar vendas

Listar vendas de um evento:

```bash
curl 'http://localhost:8080/sales-events/11111111-1111-1111-1111-111111111111/sales?page=1&pageSize=20&status=COMPLETED'
```

Filtros disponíveis:

- `page`: página atual. Padrão: `1`.
- `pageSize`: tamanho da página. Padrão: `20`, máximo: `100`.
- `status`: `PENDING_PAYMENT`, `COMPLETED` ou `FAILED`.
- `eventName`: filtro parcial pelo nome do evento.

Buscar uma venda específica dentro de um evento:

```bash
curl 'http://localhost:8080/sales-events/11111111-1111-1111-1111-111111111111/sales/generated-sale-uuid'
```

## Métricas

Endpoints:

- API: `http://localhost:8080/metrics`
- Worker: `http://localhost:9091/metrics`

Métricas iniciais:

- `http_requests_total`
- `http_request_duration_seconds`
- `sales_created_total`
- `worker_sales_processed_total`
- `worker_processing_duration_seconds`

## Testes

Os testes unitários ficam próximos do pacote testado, seguindo o padrão comum em Go.

```bash
make test
```

Estrutura atual:

- `internal/httpapi/router_test.go`: testa handlers Gin, validações HTTP e publicação de eventos usando fakes.
- `internal/httpapi/sales_store.go`: isola o SQL em um store Postgres, deixando o router testável sem banco real.
- `internal/notification`: monta e envia o email com anexos PNG de QR Code.

Próximo passo natural: adicionar testes de integração para Postgres e RabbitMQ com Docker.

## Eventos iniciais

- `SALE_CREATED`: publicado pela API após validação.
- `SALE_COMPLETED`: registrado na outbox e publicado no RabbitMQ quando o pagamento é aprovado.
- `SALE_FAILED`: registrado na outbox quando a reserva ou pagamento falha.

## Próximos passos naturais

- Publicar eventos da tabela `outbox_events`.
- Separar retry de notificações com status `FAILED` em um worker próprio.
- Adicionar migrations versionadas com ferramenta dedicada.
- Criar autenticação na API.
- Adicionar testes de integração com Postgres e RabbitMQ.
