# Tempo Ate Eventos com Analise de Sobrevivencia

Este documento descreve a atividade S1.3 da trilha estatistica do portfolio. A
pergunta estatistica e:

```text
Quanto tempo normalmente leva para uma venda avancar de etapa?
```

## Por que sobrevivencia

Em operacoes reais, nem toda venda ja terminou quando o export e gerado. Uma
venda pode estar aguardando pagamento, um email pode ainda nao ter sido enviado,
ou um ticket pode ainda nao ter feito check-in. Essas observacoes nao devem ser
tratadas como falha definitiva. Elas sao observacoes censuradas: sabemos que o
evento ainda nao aconteceu ate o tempo de corte, mas nao sabemos se acontecera
depois.

## Analises Geradas

| `eventName` | Unidade | Inicio | Evento observado | Censura |
| --- | --- | --- | --- | --- |
| `time_to_payment` | `sale` | `sale.created` | `payment.processed` com `APPROVED` | venda sem pagamento aprovado ate `generatedAt` |
| `time_to_email_sent` | `sale` | `sale.created` | `email.sent` | venda sem email enviado ate `generatedAt` |
| `time_to_check_in` | `sale_ticket` | `sale.item.created` | `checkin.completed` | ticket sem check-in ate `generatedAt` |

## Campos

Cada item em `survivalAnalyses` contem:

- `observationCount`: total de observacoes completas ou censuradas;
- `eventCount`: observacoes em que o evento aconteceu;
- `censoredCount`: observacoes em que o evento ainda nao aconteceu;
- `percentilesSeconds`: `p50`, `p90` e `p95` calculados apenas sobre eventos
  observados;
- `hazardTable`: tabela por faixa de tempo com `atRisk`, `events`, `censored`,
  `hazard` e `survivalProbability`.

## Tabela de Hazard

As faixas padrao sao:

| Faixa | Interpretacao |
| --- | --- |
| `0-60s` | eventos praticamente imediatos |
| `60-300s` | ate 5 minutos |
| `300-900s` | 5 a 15 minutos |
| `900-3600s` | 15 a 60 minutos |
| `3600s+` | mais de 1 hora |

O `hazard` da faixa e:

```text
events / atRisk
```

`survivalProbability` acumula a chance estimada de ainda nao ter observado o
evento depois de atravessar as faixas anteriores.

## Interpretacao

Exemplo operacional:

```text
time_to_payment:
observationCount = 100
eventCount = 82
censoredCount = 18
p50 = 180s
```

Leitura: entre as vendas que pagaram dentro do periodo observado, metade pagou
em ate 180 segundos. As 18 vendas censuradas ainda estavam sem pagamento no
tempo de corte; elas nao devem ser contadas como "nunca pagarao".

## Limitacoes

- Percentis ignoram censura e devem ser lidos junto com `censoredCount`.
- A tabela de hazard usa faixas fixas simples, nao um estimador Kaplan-Meier
  completo.
- `generatedAt` e o tempo de corte. Exports gerados em momentos diferentes podem
  alterar censura e duracoes para vendas ainda abertas.
