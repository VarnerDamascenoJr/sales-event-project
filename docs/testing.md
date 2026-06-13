# Testing

## Estratégia de testes identificada

- `Fato confirmado`: o projeto usa testes unitários com o pacote padrão `testing`.
- `Fato confirmado`: existe um teste de integração com build tag `integration`.
- `Fato confirmado`: não há framework externo de testes além do padrão da linguagem.

## Organização

- `internal/httpapi/router_test.go`
  - valida handlers, autenticação, rate limit, webhooks e payloads.
- `internal/outbox/publisher_test.go`
  - cobre backoff, trim de erro e roteamento de tipos de evento.
- `internal/retention/cleaner_test.go`
  - cobre retenção com fake DB.
- `internal/worker/ticket_delivery_processor_test.go`
  - cobre backoff/regras de retry de email.
- `cmd/migrate/main_test.go`
  - cobre parsing básico do runner de migrations.
- `tests/integration/sales_flow_test.go`
  - cobre fluxo ponta a ponta com serviços reais.

## Cobertura comportamental observável

### Bem coberto

- validações HTTP centrais;
- autenticação por API key;
- proteção de webhooks;
- rate limit público;
- check-in duplicado;
- backoff de outbox;
- backoff de email retry;
- parsing do runner de migration;
- fluxo de integração aprovado e falho.

### Lacunas aparentes

- `Fato confirmado`: não há testes unitários dedicados para:
  - `internal/worker/sales_processor.go`;
  - `internal/httpapi/sales_store.go` em nível de store/SQL;
  - `internal/messaging/rabbitmq.go`;
  - `internal/notification/mailer.go`;
  - `internal/observability/logging.go`.
- `Inferência`: boa parte das regras transacionais está concentrada no store PostgreSQL e depende principalmente do teste de integração para cobertura real.

## Mocks e doubles

- `Fato confirmado`: `router_test.go` usa `fakeSalesStore`, `fakePublisher` e `fakeAuthStore`.
- `Fato confirmado`: `cleaner_test.go` usa `fakeDB`.
- `Fato confirmado`: os testes de retry/backoff são puros, sem infraestrutura externa.

## Comandos disponíveis

- Testes unitários:
  - `make test`
- Cobertura:
  - `make test-cover`
- Integração:
  - `make test-integration`
- Testes diretos em pacote específico:
  - `docker run --rm -v "$(pwd)":/app -w /app golang:1.22-alpine go test ./internal/httpapi`
  - ajuste o pacote conforme necessário.

## Typecheck e lint

- `Fato confirmado`: não existe comando separado de `typecheck`; em Go isso vem embutido em `go test`/`go build`.
- `Fato confirmado`: lint usa `golangci-lint` via contêiner (`make lint`).
- `Fato confirmado`: formatação usa `gofmt` via contêiner (`make fmt`).

## Validação executada nesta análise

- `Fato confirmado`: `make test` executou com sucesso.
- `Fato confirmado`: `docker compose build api worker migrate` executou com sucesso.
- `Fato confirmado`: `go test ./...` e `go build ./...` não rodaram no host porque `go` não está instalado.
- `Fato confirmado`: os testes de integração não foram executados nesta análise.

## Padrões recomendados para novos testes

- Preferir testes unitários próximos ao pacote quando a regra for local.
- Usar doubles simples quando o alvo for o roteador HTTP.
- Usar integração quando a regra depender de transação, locking, outbox ou consistência com PostgreSQL/RabbitMQ.
- Em mudanças de pagamento, cobrir sempre:
  - venda aprovada;
  - venda falha;
  - idempotência de webhook;
  - emissão/duplicação de tickets;
  - restauração de estoque.
