# Sales Event Project

Backend moderno em Go usando Gin, RabbitMQ, PostgreSQL, Prometheus e Grafana.

O projeto modela uma venda de ingressos como fluxo orientado a eventos:

```text
Cliente -> Gin API -> RabbitMQ -> Worker Go -> PostgreSQL
                              \-> Prometheus -> Grafana
```

## Componentes

- `cmd/api`: API HTTP em Gin. Valida a venda, publica `SALE_CREATED` no RabbitMQ e responde `PROCESSING`.
- `cmd/worker`: consumidor RabbitMQ. Processa `SALE_CREATED`, reserva ingressos, simula pagamento e persiste o estado no PostgreSQL.
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

Depois disso, o worker consome `SALE_CREATED` e grava a venda como `COMPLETED`.

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

## Eventos iniciais

- `SALE_CREATED`: publicado pela API após validação.
- `SALE_COMPLETED`: registrado na outbox quando o worker conclui.
- `SALE_FAILED`: registrado na outbox quando o worker marca falha.

## Próximos passos naturais

- Separar payment worker e notification worker em filas próprias.
- Publicar eventos da tabela `outbox_events`.
- Adicionar migrations versionadas com ferramenta dedicada.
- Criar autenticação na API.
- Adicionar testes de integração com Postgres e RabbitMQ.
