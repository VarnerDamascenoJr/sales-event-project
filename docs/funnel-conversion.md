# Funil de Conversao com Incerteza

Este documento descreve a atividade S1.2 da trilha estatistica do portfolio. A
pergunta estatistica e:

```text
Qual a probabilidade de uma venda aceita virar demanda realizada?
```

## Etapas

O funil usa cinco etapas:

| Etapa | Evidencia no export analitico | Interpretacao |
| --- | --- | --- |
| `accepted` | `sale.created` ou `sale.item.created` | A venda entrou na base operacional |
| `pending_payment` | venda com status `PENDING_PAYMENT` ou `COMPLETED` | A reserva chegou ao estado pagavel |
| `paid` | `payment.processed` com status `APPROVED` | O pagamento foi aprovado |
| `ticket_issued` | `ticket.issued` | Ao menos um ticket foi emitido para a venda/tipo |
| `check_in` | `checkin.completed` | A demanda virou presenca observada |

## Unidade e Segmentacao

A unidade de analise e a combinacao `saleId + ticketId`. Uma venda com dois
tipos de ticket pode aparecer em dois segmentos de ticket, porque a pergunta
estatistica por tipo de ticket precisa preservar essa dimensao.

Os segmentos sao separados por:

- `salesEventId`;
- `ticketId` e `ticketType`;
- `provider`, usando `unknown` quando ainda nao existe provider observado;
- janela da coorte: `1m`, `5m`, `1h` ou `1d`.

A janela e definida pelo horario de aceite da venda. Isso cria coortes como
"vendas aceitas entre 10:00 e 10:05". Pagamentos, tickets emitidos e check-ins
podem acontecer depois da janela, mas continuam associados a coorte original.

## Proporcao Binomial

Cada etapa condicional e tratada como uma proporcao:

```text
conversionProbability = successes / trials
```

Exemplo:

```text
pending_payment -> paid
trials = 40 vendas que chegaram a pending_payment
successes = 32 vendas pagas
conversionProbability = 32 / 40 = 0.8
```

Quando `trials = 0`, a conversao pontual e reportada como `0` e o intervalo de
confianca fica vazio (`lower = 0`, `upper = 0`). Isso evita fingir precisao onde
nao ha amostra.

## Intervalo de Wilson

O export usa intervalo de Wilson 95% para proporcoes. Ele e preferido a formula
normal simples porque se comporta melhor quando:

- a amostra e pequena;
- a proporcao observada esta perto de `0`;
- a proporcao observada esta perto de `1`.

O campo gerado e:

```json
{
  "method": "wilson",
  "level": 0.95,
  "lower": 0.718971,
  "upper": 0.862
}
```

## Alternativa Bayesiana Simples

Tambem e reportada uma posterior Beta-Binomial com prior Beta(1,1). Essa prior
e uniforme: antes de olhar os dados, nenhuma probabilidade entre 0 e 1 e
favorecida.

Depois da observacao:

```text
posteriorAlpha = 1 + successes
posteriorBeta = 1 + failures
posteriorMean = posteriorAlpha / (posteriorAlpha + posteriorBeta)
```

Essa leitura e util em amostras pequenas porque a media posterior nao fica
exatamente `0` ou `1` apenas porque poucos eventos foram observados.

## Limitacoes

- O export usa estados atuais de algumas entidades; portanto, o status de uma
  venda pode refletir o momento do export, nao necessariamente o estado exato no
  instante original da etapa.
- O provider so e conhecido depois que ha evento de pagamento. Vendas sem
  pagamento aprovado entram em `provider = unknown`.
- Esta entrega mede incerteza de proporcoes observacionais. Ela nao estima
  efeito causal de mudancas no produto ou na operacao.
