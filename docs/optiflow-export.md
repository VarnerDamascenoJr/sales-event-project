# Exportacao para o OptiFlow

O comando `optiflow-export` transforma o historico operacional do PostgreSQL em
um JSON versionado que pode ser importado pelo `OptiFlow` como cenario de
planejamento.

## Executar

```bash
go run ./cmd/optiflow-export \
  -sales-event-id 11111111-1111-1111-1111-111111111111 \
  -output /tmp/optiflow-sales-history.json
```

O comando usa `DATABASE_URL`; quando a variavel nao existe, usa o mesmo padrao
local dos demais entrypoints:

```text
postgres://sales:sales@localhost:5432/sales_event?sslmode=disable
```

## Contrato

O documento gerado usa `schemaVersion:
sales-event-optiflow-export.v1` e inclui:

- metadados do evento de venda;
- vendas e clientes;
- itens vendidos;
- status de pagamento;
- status de email;
- contagens de tickets emitidos e check-ins;
- campos de rastreabilidade: `requestId`, `correlationId` e `transactionId`.

O `OptiFlow` importa apenas vendas concluídas por padrão. Vendas falhas ou ainda
pendentes continuam no export para auditoria, mas não viram demanda roteável.

## Limites

O banco não guarda endereço ou coordenada do cliente. Por isso, o importador do
`OptiFlow` cria localidades sintéticas determinísticas a partir do `saleId`.
Isso mantém o cenário reproduzível sem fingir precisão geográfica.
