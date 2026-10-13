package analytics

import "time"

type WindowSpec struct {
	Name     string
	Duration time.Duration
}

type Document struct {
	SchemaVersion    string             `json:"schemaVersion"`
	GeneratedAt      time.Time          `json:"generatedAt"`
	Source           Source             `json:"source"`
	Summary          Summary            `json:"summary"`
	Events           []Event            `json:"events"`
	Windows          []WindowAggregate  `json:"windows"`
	Funnels          []FunnelSegment    `json:"funnels,omitempty"`
	SurvivalAnalyses []SurvivalAnalysis `json:"survivalAnalyses,omitempty"`
	DemandForecasts  []DemandForecast   `json:"demandForecasts,omitempty"`
	StockoutRisks    []StockoutRisk     `json:"stockoutRisks,omitempty"`
	SimulationPriors *SimulationPriors  `json:"simulationPriors,omitempty"`
}

type Source struct {
	Service      string   `json:"service"`
	SalesEventID string   `json:"salesEventId,omitempty"`
	Filter       string   `json:"filter,omitempty"`
	WindowSizes  []string `json:"windowSizes"`
}

type Summary struct {
	EventCount      int            `json:"eventCount"`
	WindowCount     int            `json:"windowCount"`
	EventTypeCounts map[string]int `json:"eventTypeCounts"`
}

type Event struct {
	EventID         string    `json:"eventId,omitempty"`
	EventType       string    `json:"eventType"`
	OccurredAt      time.Time `json:"occurredAt"`
	SalesEventID    string    `json:"salesEventId,omitempty"`
	SaleID          string    `json:"saleId,omitempty"`
	TicketID        string    `json:"ticketId,omitempty"`
	TicketType      string    `json:"ticketType,omitempty"`
	Status          string    `json:"status,omitempty"`
	Provider        string    `json:"provider,omitempty"`
	OutboxEventType string    `json:"outboxEventType,omitempty"`
	AmountCents     int       `json:"amountCents,omitempty"`
	Quantity        int       `json:"quantity,omitempty"`
	Attempts        int       `json:"attempts,omitempty"`
	RequestID       string    `json:"requestId,omitempty"`
	CorrelationID   string    `json:"correlationId,omitempty"`
	TransactionID   string    `json:"transactionId,omitempty"`
}

type WindowAggregate struct {
	WindowSize  string          `json:"windowSize"`
	WindowStart time.Time       `json:"windowStart"`
	WindowEnd   time.Time       `json:"windowEnd"`
	Counts      AggregateCounts `json:"counts"`
}

type AggregateCounts struct {
	TotalEvents          int            `json:"totalEvents"`
	EventTypeCounts      map[string]int `json:"eventTypeCounts"`
	SaleStatusCounts     map[string]int `json:"saleStatusCounts,omitempty"`
	PaymentStatusCounts  map[string]int `json:"paymentStatusCounts,omitempty"`
	OutboxStatusCounts   map[string]int `json:"outboxStatusCounts,omitempty"`
	EmailStatusCounts    map[string]int `json:"emailStatusCounts,omitempty"`
	CheckInCount         int            `json:"checkInCount,omitempty"`
	TicketQuantityByType map[string]int `json:"ticketQuantityByType,omitempty"`
	TotalAmountCents     int            `json:"totalAmountCents,omitempty"`
}

const DemandForecastMethodMovingAverage = "moving_average"

type DemandForecastOptions struct {
	MovingAverageWindow int
	TestFraction        float64
	MinimumTestWindows  int
	HorizonWindows      int
	MaxSeriesWindows    int
}

type DemandForecast struct {
	SalesEventID        string                `json:"salesEventId,omitempty"`
	TicketID            string                `json:"ticketId,omitempty"`
	TicketType          string                `json:"ticketType,omitempty"`
	WindowSize          string                `json:"windowSize"`
	Method              string                `json:"method"`
	MovingAverageWindow int                   `json:"movingAverageWindow"`
	TrainWindowCount    int                   `json:"trainWindowCount"`
	TestWindowCount     int                   `json:"testWindowCount"`
	ForecastWindowStart time.Time             `json:"forecastWindowStart"`
	ForecastWindowEnd   time.Time             `json:"forecastWindowEnd"`
	ForecastQuantity    float64               `json:"forecastQuantity"`
	Metrics             DemandForecastMetrics `json:"metrics"`
	Series              []DemandSeriesPoint   `json:"series"`
}

type DemandForecastMetrics struct {
	MAE  float64 `json:"mae"`
	RMSE float64 `json:"rmse"`
}

type DemandSeriesPoint struct {
	WindowStart       time.Time `json:"windowStart"`
	WindowEnd         time.Time `json:"windowEnd"`
	Split             string    `json:"split"`
	ObservedQuantity  *int      `json:"observedQuantity,omitempty"`
	PredictedQuantity *float64  `json:"predictedQuantity,omitempty"`
	AbsoluteError     *float64  `json:"absoluteError,omitempty"`
	SquaredError      *float64  `json:"squaredError,omitempty"`
}

const (
	FunnelStageAccepted     = "accepted"
	FunnelStagePending      = "pending_payment"
	FunnelStagePaid         = "paid"
	FunnelStageTicketIssued = "ticket_issued"
	FunnelStageCheckIn      = "check_in"
)

type FunnelOptions struct {
	ConfidenceLevel float64
	WilsonZ         float64
	PriorAlpha      float64
	PriorBeta       float64
}

type FunnelSegment struct {
	SalesEventID string       `json:"salesEventId,omitempty"`
	TicketID     string       `json:"ticketId,omitempty"`
	TicketType   string       `json:"ticketType,omitempty"`
	Provider     string       `json:"provider"`
	WindowSize   string       `json:"windowSize"`
	WindowStart  time.Time    `json:"windowStart"`
	WindowEnd    time.Time    `json:"windowEnd"`
	Steps        []FunnelStep `json:"steps"`
}

type FunnelStep struct {
	From                  string             `json:"from"`
	To                    string             `json:"to"`
	Trials                int                `json:"trials"`
	Successes             int                `json:"successes"`
	ConversionProbability float64            `json:"conversionProbability"`
	ConfidenceInterval    ConfidenceInterval `json:"confidenceInterval"`
	Bayesian              BetaBinomial       `json:"bayesian"`
}

type ConfidenceInterval struct {
	Method string  `json:"method"`
	Level  float64 `json:"level"`
	Lower  float64 `json:"lower"`
	Upper  float64 `json:"upper"`
}

type BetaBinomial struct {
	PriorAlpha     float64 `json:"priorAlpha"`
	PriorBeta      float64 `json:"priorBeta"`
	PosteriorAlpha float64 `json:"posteriorAlpha"`
	PosteriorBeta  float64 `json:"posteriorBeta"`
	PosteriorMean  float64 `json:"posteriorMean"`
}

type SimulationPriors struct {
	SchemaVersion     string                     `json:"schemaVersion"`
	GeneratedAt       time.Time                  `json:"generatedAt"`
	Source            SimulationPriorSource      `json:"source"`
	Period            SimulationPriorPeriod      `json:"period"`
	SampleSize        SimulationPriorSampleSize  `json:"sampleSize"`
	Estimates         SimulationPriorEstimates   `json:"estimates"`
	Uncertainty       SimulationPriorUncertainty `json:"uncertainty"`
	ConversionStages  []ConversionStagePrior     `json:"conversionStages,omitempty"`
	Demand            []DemandPrior              `json:"demand,omitempty"`
	OperationalTiming []OperationalTimingPrior   `json:"operationalTiming,omitempty"`
	StockoutRisks     []StockoutRiskPrior        `json:"stockoutRisks,omitempty"`
}

type SimulationPriorSource struct {
	Service       string `json:"service"`
	SchemaVersion string `json:"schemaVersion"`
	SalesEventID  string `json:"salesEventId,omitempty"`
	Filter        string `json:"filter,omitempty"`
}

type SimulationPriorPeriod struct {
	StartedAt string `json:"startedAt"`
	EndedAt   string `json:"endedAt"`
}

type SimulationPriorSampleSize struct {
	SaleCount             int `json:"saleCount"`
	CompletedSaleCount    int `json:"completedSaleCount"`
	CancelledSaleCount    int `json:"cancelledSaleCount"`
	CompletedDemandCount  int `json:"completedDemandCount"`
	TotalItemQuantity     int `json:"totalItemQuantity"`
	CompletedItemQuantity int `json:"completedItemQuantity"`
}

type SimulationPriorEstimates struct {
	ConversionProbability        float64 `json:"conversionProbability"`
	CancellationProbability      float64 `json:"cancellationProbability"`
	DemandMean                   float64 `json:"demandMean"`
	DemandStdDev                 float64 `json:"demandStdDev"`
	DemandCoefficientOfVariation float64 `json:"demandCoefficientOfVariation"`
	ModalDemand                  int     `json:"modalDemand"`
	DemandDeviationProbability   float64 `json:"demandDeviationProbability"`
}

type SimulationPriorUncertainty struct {
	CancellationProbability    float64 `json:"cancellationProbability"`
	DemandVariationProbability float64 `json:"demandVariationProbability"`
	DemandVariationRate        float64 `json:"demandVariationRate"`
}

type ConversionStagePrior struct {
	From                  string             `json:"from"`
	To                    string             `json:"to"`
	Trials                int                `json:"trials"`
	Successes             int                `json:"successes"`
	ConversionProbability float64            `json:"conversionProbability"`
	ConfidenceInterval    ConfidenceInterval `json:"confidenceInterval"`
	Bayesian              BetaBinomial       `json:"bayesian"`
}

type DemandPrior struct {
	SalesEventID     string                `json:"salesEventId,omitempty"`
	TicketID         string                `json:"ticketId,omitempty"`
	TicketType       string                `json:"ticketType,omitempty"`
	WindowSize       string                `json:"windowSize"`
	Method           string                `json:"method"`
	ObservedWindows  int                   `json:"observedWindows"`
	ForecastQuantity float64               `json:"forecastQuantity"`
	Metrics          DemandForecastMetrics `json:"metrics"`
}

type OperationalTimingPrior struct {
	EventName        string             `json:"eventName"`
	FromStage        string             `json:"fromStage"`
	ToStage          string             `json:"toStage"`
	UnitOfAnalysis   string             `json:"unitOfAnalysis"`
	ObservationCount int                `json:"observationCount"`
	EventCount       int                `json:"eventCount"`
	CensoredCount    int                `json:"censoredCount"`
	Percentiles      map[string]float64 `json:"percentilesSeconds,omitempty"`
}

type StockoutRiskPrior struct {
	SalesEventID        string  `json:"salesEventId,omitempty"`
	TicketID            string  `json:"ticketId"`
	TicketType          string  `json:"ticketType,omitempty"`
	AvailableQuantity   int     `json:"availableQuantity"`
	WindowSize          string  `json:"windowSize"`
	HorizonWindowCount  int     `json:"horizonWindowCount"`
	ExpectedDemand      float64 `json:"expectedDemand"`
	StockoutProbability float64 `json:"stockoutProbability"`
	RiskBand            string  `json:"riskBand"`
	Status              string  `json:"status"`
}

const (
	StockoutRiskMethodPoissonRate = "poisson_rate"

	StockoutRiskStatusEstimated           = "estimated"
	StockoutRiskStatusInsufficientHistory = "insufficient_history"
	StockoutRiskStatusNoObservedDemand    = "no_observed_demand"
	StockoutRiskStatusStockout            = "stockout"

	StockoutRiskBandLow      = "low"
	StockoutRiskBandMedium   = "medium"
	StockoutRiskBandHigh     = "high"
	StockoutRiskBandCritical = "critical"
)

type TicketInventory struct {
	SalesEventID      string `json:"salesEventId,omitempty"`
	TicketID          string `json:"ticketId"`
	TicketType        string `json:"ticketType,omitempty"`
	AvailableQuantity int    `json:"availableQuantity"`
}

type StockoutRiskOptions struct {
	WindowSize             string
	HorizonWindows         int
	MinimumObservedWindows int
	MaxSeriesWindows       int
}

type StockoutRisk struct {
	SalesEventID              string                     `json:"salesEventId,omitempty"`
	TicketID                  string                     `json:"ticketId"`
	TicketType                string                     `json:"ticketType,omitempty"`
	AvailableQuantity         int                        `json:"availableQuantity"`
	WindowSize                string                     `json:"windowSize"`
	HorizonWindowCount        int                        `json:"horizonWindowCount"`
	HorizonStart              time.Time                  `json:"horizonStart"`
	HorizonEnd                time.Time                  `json:"horizonEnd"`
	Method                    string                     `json:"method"`
	Status                    string                     `json:"status"`
	RiskBand                  string                     `json:"riskBand"`
	ObservedWindowCount       int                        `json:"observedWindowCount"`
	ObservedDemandQuantity    int                        `json:"observedDemandQuantity"`
	MeanDemandPerWindow       float64                    `json:"meanDemandPerWindow"`
	VarianceDemandPerWindow   float64                    `json:"varianceDemandPerWindow"`
	ExpectedDemand            float64                    `json:"expectedDemand"`
	StockoutProbability       float64                    `json:"stockoutProbability"`
	ExpectedWindowsToStockout *float64                   `json:"expectedWindowsToStockout,omitempty"`
	ExpectedStockoutAt        *time.Time                 `json:"expectedStockoutAt,omitempty"`
	Distribution              []StockoutProbabilityPoint `json:"distribution,omitempty"`
}

type StockoutProbabilityPoint struct {
	WindowOffset             int       `json:"windowOffset"`
	WindowStart              time.Time `json:"windowStart"`
	WindowEnd                time.Time `json:"windowEnd"`
	CumulativeExpectedDemand float64   `json:"cumulativeExpectedDemand"`
	StockoutProbability      float64   `json:"stockoutProbability"`
}
