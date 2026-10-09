# Event Analytics Export

The `analytics-export` command turns the PostgreSQL operational history into a
versioned statistical dataset. Unlike `optiflow-export`, which generates a
planning scenario for `OptiFlow`, this export preserves individual events and
windowed aggregates for statistical analysis.

## Run

```bash
go run ./cmd/analytics-export \
  -sales-event-id 11111111-1111-1111-1111-111111111111 \
  -output /tmp/sales-analytics-export.json
```

Optional time filters use RFC3339:

```bash
go run ./cmd/analytics-export \
  -start 2026-09-01T10:00:00Z \
  -end 2026-09-02T00:00:00Z
```

The command uses `DATABASE_URL`; when the variable is not present, it uses the
same local default as the other entrypoints:

```text
postgres://sales:sales@localhost:5432/sales_event?sslmode=disable
```

## HTTP API

The same contract is also available to dashboards through the protected
endpoint:

```text
GET /analytics/export?salesEventId=&start=&end=&limit=
```

Authentication:

- required `X-API-Key` header;
- allowed roles: `SUPPORT` or `ADMIN`;
- local keys seeded by the migrations:
  - `dev-support-key`: `SUPPORT` role;
  - `dev-admin-key`: `ADMIN` role.

Local example:

```bash
curl "http://localhost:8080/analytics/export?salesEventId=11111111-1111-1111-1111-111111111111&start=2026-09-01T10:00:00Z&end=2026-09-02T00:00:00Z&limit=2000" \
  -H "X-API-Key: dev-support-key"
```

To generate a large dataset before exporting:

```bash
scripts/generate-analytics-demo-data.sh --sales 50 --reset-demo-inventory
```

The script uses HTTP endpoints and, locally, queries Postgres through
`docker compose exec` only to select issued tickets that will be used for
check-ins.

Parameters:

- `salesEventId`: optional filter by sales event;
- `start`: inclusive lower bound in RFC3339;
- `end`: exclusive upper bound in RFC3339;
- `limit`: maximum number of individual events returned. The default is `2000`,
  and the maximum accepted by the HTTP API is `10000`.

Expected responses:

- `200`: `sales-analytics-export.v1` document;
- `400`: invalid filter, such as `limit` out of range or a timestamp outside
  RFC3339;
- `401`: missing or invalid `X-API-Key`;
- `403`: valid key without the `SUPPORT` or `ADMIN` role;
- `500`: internal failure while exporting data.

## Contract

The generated document uses `schemaVersion: sales-analytics-export.v1` and
includes:

- individual events with timestamps normalized to UTC;
- aggregates over `1m`, `5m`, `1h`, and `1d` windows;
- counts by event type;
- counts by sale, payment, outbox, and email status;
- check-in count;
- ticket quantities by type when the dimension exists;
- baseline demand forecast by window and ticket type when there is enough time
  series data;
- baseline stockout risk by ticket, combining current stock with the historical
  demand rate;
- simulation priors for `OptiFlow`, derived from the same analytics dataset;
- traceability fields: `requestId`, `correlationId`, and `transactionId`.

## Exported Events

| Event | Source | Note |
| --- | --- | --- |
| `sale.created` | `sales.created_at` | Represents a sale persisted by the worker |
| `sale.item.created` | `sale_items.created_at` | Preserves demand by ticket type |
| `payment.processed` | `payments.processed_at` | Includes status, provider, and amount |
| `outbox.created` | `outbox_events.created_at` | Includes event type, status, and attempts |
| `outbox.published` | `outbox_events.published_at` | Exported only when published |
| `email.status` | `email_notifications.created_at` | Current operational state of the send |
| `email.sent` | `email_notifications.sent_at` | Exported only when a timestamp exists |
| `email.delivered` | `email_notifications.delivered_at` | Exported only when a timestamp exists |
| `email.opened` | `email_notifications.opened_at` | Exported only when a timestamp exists |
| `email.clicked` | `email_notifications.clicked_at` | Exported only when a timestamp exists |
| `email.bounced` | `email_notifications.bounced_at` | Exported only when a timestamp exists |
| `ticket.issued` | `issued_tickets.created_at` | Counts tickets issued after approved payment |
| `checkin.completed` | `ticket_check_ins.checked_in_at` | Counts demand realized at the event |

## Funnel With Uncertainty

The `funnels` field summarizes conditional conversions by `salesEventId`,
`ticketType`, `provider`, and cohort window. The cohort is defined by the
`sale.created` timestamp; later events count as success within the same cohort.

The stages are:

```text
accepted -> pending_payment -> paid -> ticket_issued -> check_in
```

Each stage reports:

- `trials`: number of items that reached the previous stage;
- `successes`: number of items that reached the next stage;
- `conversionProbability`: `successes / trials`;
- `confidenceInterval`: Wilson interval for a binomial proportion;
- `bayesian`: Beta-Binomial posterior with a Beta(1,1) prior.

The Wilson interval avoids misleading intervals when the sample is small or when
the observed conversion is zero or one. The Bayesian alternative uses a uniform
Beta(1,1) prior: before observing data, all probabilities between 0 and 1 are
treated as equally plausible. After observing `successes` and `failures`, the
posterior becomes `Beta(1 + successes, 1 + failures)`.

## Survival Analysis

The `survivalAnalyses` field measures time to operational events and separates
complete observations from censored ones. A censored observation is a sale or
ticket that has not reached the event by `generatedAt`, which acts as the export
cutoff time.

Generated analyses:

| Analysis | Unit | Start | Observed event |
| --- | --- | --- | --- |
| `time_to_payment` | `sale` | accepted sale | approved `payment.processed` |
| `time_to_email_sent` | `sale` | accepted sale | `email.sent` |
| `time_to_check_in` | `sale_ticket` | accepted sale item | `checkin.completed` |

Each analysis includes:

- `observationCount`, `eventCount`, and `censoredCount`;
- `p50`, `p90`, and `p95` percentiles in seconds, calculated only from observed
  events;
- `hazardTable`, with duration buckets, number at risk, events, censored
  observations, simple hazard, and cumulative survival probability.

The default buckets are `0-60s`, `60-300s`, `300-900s`, `900-3600s`, and
`3600s+`.

## Baseline Demand Forecast

The `demandForecasts` field estimates future demand by `salesEventId`, ticket,
ticket type, and window size. Observed demand comes from `sale.item.created`
events, using the purchased quantity per sale item.

The initial baseline uses a temporal moving average:

- the series is sorted by time and densified with empty windows between the
  first and last observed points;
- densified series with more than 500 windows per ticket are omitted to avoid
  accidentally large payloads;
- the final windows are split out as a temporal test set, without shuffling;
- each test point receives a one-step-ahead forecast using only previous
  windows;
- `mae` and `rmse` measure out-of-sample error;
- the next window receives `forecastQuantity`.

Main fields:

| Field | Interpretation |
| --- | --- |
| `method` | Method used; currently `moving_average` |
| `movingAverageWindow` | Maximum number of previous windows used in the average |
| `trainWindowCount` | Windows used before the test period |
| `testWindowCount` | Windows evaluated out of sample |
| `forecastWindowStart` / `forecastWindowEnd` | Next forecast horizon |
| `forecastQuantity` | Expected quantity for the next window |
| `metrics.mae` | Mean absolute error on the temporal test |
| `metrics.rmse` | Root mean squared error on the temporal test |
| `series` | `train`, `test`, and `forecast` points with observed and forecast values |

## Stockout Risk

The `stockoutRisks` field estimates, by ticket, the chance that current stock
will run out within a short operational horizon. Stock comes from
`tickets.available_quantity` at export time. Historical demand comes from
`sale.item.created` events filtered by the same `salesEventId` and time range as
the export.

The initial baseline uses a Poisson distribution over the average demand rate:

- the demand series is densified into `5m` windows;
- the default horizon is 12 windows, or 1 hour;
- the average rate per window is multiplied by the horizon to obtain expected
  demand;
- stockout probability is `P(future demand > availableQuantity)`;
- `distribution` shows the cumulative stockout probability at each future
  window;
- tickets without observed demand receive the `no_observed_demand` status;
- tickets with current stock less than or equal to zero receive the `stockout`
  status.

Main fields:

| Field | Interpretation |
| --- | --- |
| `availableQuantity` | Current ticket stock in the database |
| `windowSize` | Window used to estimate demand rate; currently `5m` |
| `horizonWindowCount` | Number of future windows considered |
| `horizonStart` / `horizonEnd` | Time range analyzed from `generatedAt` |
| `meanDemandPerWindow` | Historical average sales rate per window |
| `varianceDemandPerWindow` | Sample variance of demand by window |
| `expectedDemand` | Expected cumulative demand over the horizon |
| `stockoutProbability` | Probability of stockout within the horizon |
| `expectedWindowsToStockout` | Average time to stockout, in number of windows |
| `expectedStockoutAt` | Timestamp derived from `expectedWindowsToStockout` |
| `riskBand` | `low`, `medium`, `high`, or `critical` band |
| `status` | `estimated`, `insufficient_history`, `no_observed_demand`, or `stockout` |

## Priors For OptiFlow Simulation

The `simulationPriors` field summarizes estimated parameters for `OptiFlow` in
an `optiflow-sales-priors.v1` subdocument. It lets the simulation use observed
data without relying on manual constants.

Sources used:

- `events`: sales sample, observed period, demand per sale, and
  cancellation/non-completion rate;
- `funnels`: conditional probabilities by funnel stage, with Wilson and
  Beta-Binomial;
- `demandForecasts`: demand parameters by ticket and window;
- `survivalAnalyses`: operational timing uncertainty with preserved censoring;
- `stockoutRisks`: estimated stockout risk by ticket.

Main fields:

| Field | Interpretation |
| --- | --- |
| `sampleSize` | Sample size used to estimate conversion and demand |
| `estimates.conversionProbability` | Share of observed sales that reached approved payment |
| `estimates.cancellationProbability` | Complementary share used as cancellation uncertainty |
| `estimates.demandMean` | Average demand per completed sale with observed items |
| `uncertainty` | Parameters directly applicable by the `OptiFlow` Monte Carlo simulation |
| `conversionStages` | Priors aggregated by funnel transition |
| `demand` | Demand priors by ticket, window, and forecast method |
| `operationalTiming` | Operational timing priors derived from survival analysis |
| `stockoutRisks` | Stock risk priors by ticket |

## Fixture

A small fixture with two 5-minute windows is available at:

- `tests/fixtures/sales-analytics-export.v1.json`

It is synthetic and exists to validate the export contract, not to represent a
real operational conclusion.

## Limitations

- Some events are derived from tables that store the entity's current state. For
  example, `sale.created` uses `sales.created_at`, but the sale status is the
  status observed at export time.
- Email events combine the current operational state with specific event
  timestamps when they exist. Funnel analyses should state this rule before
  interpreting conversions.
- Segments by `provider` use `unknown` when the sale does not have an observed
  provider yet. This avoids mixing sales without confirmed payment into known
  providers.
- Survival percentiles ignore censored observations. The hazard table preserves
  the censoring count so the interpretation does not confuse "has not happened
  yet" with "will never happen."
- Windows without events are not materialized in JSON; consumers should create
  empty windows when they need dense time series.
- The demand forecast internally densifies windows without sales between the
  first and last observed points. Even so, it is a naive baseline: it serves as
  an initial reference, not as a calibrated seasonal model.
- Stockout risk uses current stock combined with historical demand from the
  active filter. When narrow time filters are used, the demand rate can become
  unstable or insufficient to estimate real risk.
