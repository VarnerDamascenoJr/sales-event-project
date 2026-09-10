# Project Structure

## Diretórios principais

### `cmd/`

- `cmd/api`
  - entrypoint da API HTTP.
- `cmd/worker`
  - entrypoint do worker principal e do servidor de métricas do worker.
- `cmd/email-retry-worker`
  - entrypoint dedicado ao retry de notificacoes de email.
- `cmd/migrate`
  - executor customizado de migrations.

### `internal/`

- `internal/config`
  - leitura e parsing de variáveis de ambiente.
- `internal/database`
  - criação do pool PostgreSQL.
- `internal/events`
  - constantes de status, limites e payloads de eventos.
- `internal/httpapi`
  - roteador, validações, autenticação, rate limit e store PostgreSQL para casos de uso expostos via HTTP.
- `internal/messaging`
  - cliente RabbitMQ, declaração de exchange/queues e publish/consume.
- `internal/metrics`
  - métricas Prometheus e middleware Gin.
- `internal/notification`
  - envio de email por SMTP ou logger fallback.
- `internal/observability`
  - configuração do logger estruturado.
- `internal/outbox`
  - publicador de eventos pendentes da tabela outbox.
- `internal/retention`
  - limpeza periódica de dados temporários.
- `internal/worker`
  - processadores assíncronos para reserva de venda e emissão/envio de tickets.

### `migrations/`

- migrations SQL versionadas do schema.
- seed local do evento/tickets.
- criação de API keys locais.
- evolução de retry/outbox/email events/payment intents.

### `tests/`

- `tests/integration`
  - teste de fluxo end-to-end com build tag `integration`.

### `deployments/`

- `prometheus`
  - configuração de scrape.
- `loki`
  - configuração de logs.
- `promtail`
  - configuração de coleta de logs Docker.
- `grafana`
  - provisioning de datasource e dashboard.

### `scripts/`

- `scripts/git-hooks/pre-commit`
  - roda `make lint-fix` e `make test`, e falha se houver diff depois da execução.

## Arquivos arquiteturalmente relevantes

- `README.md`
  - visão geral operacional e exemplos de uso.
- `go.mod`
  - módulo, versão Go e dependências.
- `Dockerfile`
  - build multi-stage de `api`, `worker`, `email-retry-worker` e `migrate`.
- `docker-compose.yml`
  - stack local completa.
- `.env.example`
  - nomes das variáveis de ambiente suportadas.
- `Makefile`
  - comandos de desenvolvimento, lint, testes e build.

## Responsabilidades por pacote

## `internal/httpapi`

- Handlers:
  - definidos diretamente em `router.go`.
- Middlewares:
  - autenticação por API key em `auth.go`;
  - webhook secret em `webhook_auth.go`;
  - assinatura HMAC do corpo em `signature_auth.go`;
  - rate limit em `rate_limit.go`;
  - métricas em `internal/metrics`.
- Store:
  - `PostgresSalesStore` e `PostgresAuthStore`.
- Segurança e validação:
  - regras de payload ficam majoritariamente em `router.go`;
  - autenticação por API key usa `auth.go` + `auth_store.go`;
  - autenticação de webhook usa `webhook_auth.go` e `signature_auth.go`.

## `internal/worker`

- `SalesProcessor`
  - traduz evento `SALE_CREATED` em reserva persistida.
- `TicketDeliveryProcessor`
  - emite QR codes, registra email pendente, envia tickets e fornece a rotina usada pelo worker dedicado de retry.

## `internal/outbox`

- `Publisher`
  - varre `outbox_events` e publica em RabbitMQ com retry/backoff.

## `internal/messaging`

- `RabbitMQ`
  - declara exchange principal, DLX e filas configuradas;
  - publica JSON;
  - expõe consumo com `Qos(10)`.

## `internal/notification`

- `SMTPSender`
  - envio real.
- `LogSender`
  - fallback quando SMTP não está configurado.

## Organização dos testes

- Unitários próximos ao código:
  - `internal/httpapi/router_test.go`
  - `internal/outbox/publisher_test.go`
  - `internal/retention/cleaner_test.go`
  - `internal/worker/ticket_delivery_processor_test.go`
  - `cmd/migrate/main_test.go`
- Integração:
  - `tests/integration/sales_flow_test.go`

## Itens não encontrados

- `Fato confirmado`: não há `pkg/`, `api/`, `service/`, `repository/` ou `domain/` separados como camadas formais.
- `Fato confirmado`: não foram encontrados arquivos de CI/CD, Terraform, Helm, Kubernetes, Serverless ou CloudFormation.
- `Fato confirmado`: não foram encontrados arquivos de configuração dedicados para `golangci-lint`; o projeto aparenta usar configuração padrão da imagem.
