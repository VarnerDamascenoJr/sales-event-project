# Integrations

## PostgreSQL

- Finalidade:
  - persistência principal.
- Módulos:
  - `internal/database`
  - `internal/httpapi/sales_store.go`
  - `internal/worker/*.go`
  - `internal/outbox/publisher.go`
  - `cmd/migrate/main.go`
- Protocolo:
  - conexão PostgreSQL via `pgx`.
- Autenticação:
  - via `DATABASE_URL`.
- Timeout/comportamento:
  - timeouts de contexto são definidos nos entrypoints e em processamentos específicos.
- Impacto de indisponibilidade:
  - API e worker encerram ao iniciar se não conseguirem conectar.

## RabbitMQ

- Finalidade:
  - transporte de eventos internos.
- Módulo:
  - `internal/messaging/rabbitmq.go`
- Protocolo:
  - AMQP 0.9.1.
- Variáveis:
  - `RABBITMQ_URL`
  - `RABBITMQ_EXCHANGE`
  - `SALES_CREATED_QUEUE`
  - `SALE_COMPLETED_QUEUE`
- Comportamento:
  - declara exchange principal e DLX;
  - declara `sales.dlq`;
  - conecta com retry no startup.
- Publicação:
  - usa `PublishWithContext` com `mandatory=true`;
  - habilita publisher confirms e trata `nack`/mensagem nao roteavel como erro.
- Retry:
  - `ConnectWithRetry(..., 20, 2*time.Second)`.
- Impacto de indisponibilidade:
  - API não sobe sem broker;
  - worker não sobe sem broker;
  - falha de publish direto em `/sales` retorna `503`.

## Email/SMTP

- Finalidade:
  - envio de tickets com QR Code.
- Módulo:
  - `internal/notification/mailer.go`
- Protocolo:
  - SMTP.
- Variáveis:
  - `SMTP_HOST`
  - `SMTP_PORT`
  - `SMTP_USERNAME`
  - `SMTP_PASSWORD`
  - `SMTP_FROM`
- Timeout:
  - não há timeout explícito de socket SMTP no código.
- Fallback:
  - se `SMTP_HOST` estiver vazio, usa `LogSender`.
- Retry:
  - sim, via `email_notifications`.
- Impacto de indisponibilidade:
  - venda concluída continua, mas envio pode falhar e entrar em retry/`DEAD_LETTER`.

## Webhook de pagamento

- Finalidade:
  - confirmação assíncrona do pagamento.
- Módulo:
  - `internal/httpapi/signature_auth.go`
  - `internal/httpapi/router.go`
  - `internal/httpapi/sales_store.go`
- Protocolo:
  - HTTP JSON.
- Autenticação:
  - HMAC SHA-256 do corpo em `X-Webhook-Signature`.
- Variáveis:
  - `PAYMENT_WEBHOOK_SECRET`
- Retry/fallback:
  - não há client nem retry ativo no projeto; o sistema apenas recebe o webhook.
- Impacto de indisponibilidade:
  - vendas podem permanecer em `PENDING_PAYMENT`.

## Webhook de eventos de email

- Finalidade:
  - rastrear entrega, abertura, clique e bounce.
- Módulo:
  - `internal/httpapi/webhook_auth.go`
  - `internal/httpapi/router.go`
- Protocolo:
  - HTTP JSON.
- Autenticação:
  - header `X-Webhook-Secret`.
- Variáveis:
  - `EMAIL_WEBHOOK_SECRET`
- Impacto de indisponibilidade:
  - rastreamento operacional fica desatualizado, mas não impede venda/pagamento.

## Prometheus

- Finalidade:
  - coleta de métricas.
- Módulos:
  - `internal/metrics`
  - endpoints `/metrics` na API e no worker.
- Configuração:
  - `deployments/prometheus/prometheus.yml`
- Variáveis:
  - `METRICS_PROTECTED`
  - `WORKER_METRICS_HOST`
  - `WORKER_METRICS_PORT`
- Impacto de indisponibilidade:
  - perda de observabilidade, sem impacto direto no domínio.

## Loki / Promtail / Grafana

- Finalidade:
  - logs centralizados e dashboards.
- Configuração:
  - `deployments/loki/loki.yml`
  - `deployments/promtail/promtail.yml`
  - `deployments/grafana/**`
- Impacto de indisponibilidade:
  - perda de visibilidade operacional.

## Integrações ou adapters não encontrados

- `Fato confirmado`: não há client HTTP para gateway de pagamento.
- `Fato confirmado`: não há SDK de provedor de email externo além de SMTP genérico.
- `Fato confirmado`: não há Redis, cache, S3, filas cloud gerenciadas ou tracing vendor.

## Riscos de integração

- `Fato confirmado`: `sale.failed` não tem fila declarada/bindada.
- `Fato confirmado`: mensagens falhas podem acabar em `sales.dlq`, mas não há consumidor nem procedimento automatizado para reprocessamento.
- `Fato confirmado`: `sale.failed` continua sem binding declarado no ambiente local, mas a publicacao obrigatoria agora transforma a falta de rota em retry/dead-letter da outbox.
- `Fato confirmado`: o servidor de métricas do worker fica exposto em `0.0.0.0` em `development` por default.
