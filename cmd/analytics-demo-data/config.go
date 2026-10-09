package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultSalesEventID       = "11111111-1111-1111-1111-111111111111"
	defaultGeneralTicketID    = "22222222-2222-2222-2222-222222222222"
	defaultVIPTicketID        = "33333333-3333-3333-3333-333333333333"
	defaultPaymentAPIKey      = "dev-payment-provider-key"
	defaultCheckInAPIKey      = "dev-check-in-key"
	defaultEmailWebhookSecret = "change-me-email-webhook-secret"
)

type config struct {
	APIBaseURL               string        `json:"apiBaseUrl"`
	SalesEventID             string        `json:"salesEventId"`
	Sales                    int           `json:"sales"`
	FailedPaymentRate        float64       `json:"failedPaymentRate"`
	CheckIns                 int           `json:"checkIns"`
	EmailEvents              bool          `json:"emailEvents"`
	ResetDemoInventory       bool          `json:"resetDemoInventory"`
	GeneralTicketID          string        `json:"generalTicketId"`
	VIPTicketID              string        `json:"vipTicketId"`
	GeneralPrice             int           `json:"generalPrice"`
	VIPPrice                 int           `json:"vipPrice"`
	GeneralStock             int           `json:"generalStock"`
	VIPStock                 int           `json:"vipStock"`
	PaymentAPIKey            string        `json:"paymentApiKey"`
	CheckInAPIKey            string        `json:"checkInApiKey"`
	EmailWebhookSecret       string        `json:"emailWebhookSecret"`
	RateLimitSleep           time.Duration `json:"-"`
	RequestTimeout           time.Duration `json:"-"`
	PaymentWaitAttempts      int           `json:"paymentWaitAttempts"`
	IssuedTicketWaitAttempts int           `json:"issuedTicketWaitAttempts"`
	RunID                    string        `json:"runId"`
	Seed                     int64         `json:"seed"`
	RateLimitSleepSeconds    float64       `json:"rateLimitSleepSeconds"`
	RequestTimeoutSeconds    float64       `json:"requestTimeoutSeconds"`
}

type safeConfig struct {
	APIBaseURL               string  `json:"apiBaseUrl"`
	SalesEventID             string  `json:"salesEventId"`
	Sales                    int     `json:"sales"`
	FailedPaymentRate        float64 `json:"failedPaymentRate"`
	CheckIns                 int     `json:"checkIns"`
	EmailEvents              bool    `json:"emailEvents"`
	ResetDemoInventory       bool    `json:"resetDemoInventory"`
	GeneralTicketID          string  `json:"generalTicketId"`
	VIPTicketID              string  `json:"vipTicketId"`
	GeneralPrice             int     `json:"generalPrice"`
	VIPPrice                 int     `json:"vipPrice"`
	GeneralStock             int     `json:"generalStock"`
	VIPStock                 int     `json:"vipStock"`
	PaymentAPIKey            string  `json:"paymentApiKey"`
	CheckInAPIKey            string  `json:"checkInApiKey"`
	EmailWebhookSecret       string  `json:"emailWebhookSecret"`
	RateLimitSleepSeconds    float64 `json:"rateLimitSleepSeconds"`
	RequestTimeoutSeconds    float64 `json:"requestTimeoutSeconds"`
	PaymentWaitAttempts      int     `json:"paymentWaitAttempts"`
	IssuedTicketWaitAttempts int     `json:"issuedTicketWaitAttempts"`
	RunID                    string  `json:"runId"`
	Seed                     int64   `json:"seed"`
}

func parseConfig(args []string, now time.Time) (config, error) {
	cfg := defaultConfig()
	flags := flag.NewFlagSet("analytics-demo-data", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	noEmailEvents := false

	flags.StringVar(&cfg.APIBaseURL, "api-base-url", cfg.APIBaseURL, "Sales API base URL")
	flags.StringVar(&cfg.SalesEventID, "sales-event-id", cfg.SalesEventID, "sales event UUID")
	flags.IntVar(&cfg.Sales, "sales", cfg.Sales, "number of sales to create")
	flags.Float64Var(&cfg.FailedPaymentRate, "failed-payment-rate", cfg.FailedPaymentRate, "payment failure rate from 0 to 1")
	flags.IntVar(&cfg.CheckIns, "check-ins", cfg.CheckIns, "maximum issued tickets to check in")
	flags.BoolVar(&cfg.EmailEvents, "email-events", cfg.EmailEvents, "emit email webhook events")
	flags.BoolVar(&noEmailEvents, "no-email-events", false, "disable email webhook events")
	flags.BoolVar(&cfg.ResetDemoInventory, "reset-demo-inventory", false, "reset seeded demo ticket inventory")
	flags.StringVar(&cfg.GeneralTicketID, "general-ticket-id", cfg.GeneralTicketID, "general ticket UUID")
	flags.StringVar(&cfg.VIPTicketID, "vip-ticket-id", cfg.VIPTicketID, "VIP ticket UUID")
	flags.IntVar(&cfg.GeneralPrice, "general-price", cfg.GeneralPrice, "general ticket unit price in cents")
	flags.IntVar(&cfg.VIPPrice, "vip-price", cfg.VIPPrice, "VIP ticket unit price in cents")
	flags.IntVar(&cfg.GeneralStock, "general-stock", cfg.GeneralStock, "general ticket inventory reset value")
	flags.IntVar(&cfg.VIPStock, "vip-stock", cfg.VIPStock, "VIP ticket inventory reset value")
	flags.StringVar(&cfg.PaymentAPIKey, "payment-api-key", cfg.PaymentAPIKey, "payment provider API key")
	flags.StringVar(&cfg.CheckInAPIKey, "check-in-api-key", cfg.CheckInAPIKey, "check-in API key")
	flags.StringVar(&cfg.EmailWebhookSecret, "email-webhook-secret", cfg.EmailWebhookSecret, "email webhook secret")
	flags.Float64Var(&cfg.RateLimitSleepSeconds, "rate-limit-sleep-seconds", cfg.RateLimitSleepSeconds, "sleep base for 429 retries")
	flags.Float64Var(&cfg.RequestTimeoutSeconds, "request-timeout-seconds", cfg.RequestTimeoutSeconds, "HTTP request timeout")
	flags.IntVar(&cfg.PaymentWaitAttempts, "payment-wait-attempts", cfg.PaymentWaitAttempts, "payment endpoint retry attempts")
	flags.IntVar(&cfg.IssuedTicketWaitAttempts, "issued-ticket-wait-attempts", cfg.IssuedTicketWaitAttempts, "issued ticket retry attempts")
	flags.StringVar(&cfg.RunID, "run-id", "", "stable run identifier")
	flags.Int64Var(&cfg.Seed, "seed", cfg.Seed, "random seed")

	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	if noEmailEvents {
		cfg.EmailEvents = false
	}
	if cfg.RunID == "" {
		cfg.RunID = now.Format("20060102150405")
	}
	if err := validateConfig(cfg); err != nil {
		return config{}, err
	}
	cfg.APIBaseURL = strings.TrimRight(cfg.APIBaseURL, "/")
	cfg.RateLimitSleep = durationFromSeconds(cfg.RateLimitSleepSeconds)
	cfg.RequestTimeout = durationFromSeconds(cfg.RequestTimeoutSeconds)
	return cfg, nil
}

func defaultConfig() config {
	return config{
		APIBaseURL:               "http://localhost:8080",
		SalesEventID:             defaultSalesEventID,
		Sales:                    50,
		FailedPaymentRate:        0.15,
		CheckIns:                 20,
		EmailEvents:              true,
		GeneralTicketID:          defaultGeneralTicketID,
		VIPTicketID:              defaultVIPTicketID,
		GeneralPrice:             10000,
		VIPPrice:                 25000,
		GeneralStock:             1000,
		VIPStock:                 500,
		PaymentAPIKey:            defaultPaymentAPIKey,
		CheckInAPIKey:            defaultCheckInAPIKey,
		EmailWebhookSecret:       defaultEmailWebhookSecret,
		RateLimitSleepSeconds:    0.4,
		RequestTimeoutSeconds:    10,
		PaymentWaitAttempts:      20,
		IssuedTicketWaitAttempts: 30,
		Seed:                     20261009,
	}
}

func validateConfig(cfg config) error {
	switch {
	case cfg.Sales <= 0:
		return errors.New("sales must be greater than zero")
	case cfg.FailedPaymentRate < 0 || cfg.FailedPaymentRate > 1:
		return errors.New("failed-payment-rate must be between 0 and 1")
	case cfg.CheckIns < 0:
		return errors.New("check-ins must be greater than or equal to zero")
	case cfg.GeneralPrice <= 0:
		return errors.New("general-price must be greater than zero")
	case cfg.VIPPrice <= 0:
		return errors.New("vip-price must be greater than zero")
	case cfg.GeneralStock < 0:
		return errors.New("general-stock must be greater than or equal to zero")
	case cfg.VIPStock < 0:
		return errors.New("vip-stock must be greater than or equal to zero")
	case cfg.RateLimitSleepSeconds <= 0:
		return errors.New("rate-limit-sleep-seconds must be greater than zero")
	case cfg.RequestTimeoutSeconds <= 0:
		return errors.New("request-timeout-seconds must be greater than zero")
	case cfg.PaymentWaitAttempts <= 0:
		return errors.New("payment-wait-attempts must be greater than zero")
	case cfg.IssuedTicketWaitAttempts <= 0:
		return errors.New("issued-ticket-wait-attempts must be greater than zero")
	}
	if err := validateUUID("sales-event-id", cfg.SalesEventID); err != nil {
		return err
	}
	if err := validateUUID("general-ticket-id", cfg.GeneralTicketID); err != nil {
		return err
	}
	if err := validateUUID("vip-ticket-id", cfg.VIPTicketID); err != nil {
		return err
	}
	return nil
}

func validateUUID(name string, value string) error {
	if _, err := uuid.Parse(value); err != nil {
		return fmt.Errorf("%s must be a UUID: %w", name, err)
	}
	return nil
}

func durationFromSeconds(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}

func redactConfig(cfg config) safeConfig {
	return safeConfig{
		APIBaseURL:               cfg.APIBaseURL,
		SalesEventID:             cfg.SalesEventID,
		Sales:                    cfg.Sales,
		FailedPaymentRate:        cfg.FailedPaymentRate,
		CheckIns:                 cfg.CheckIns,
		EmailEvents:              cfg.EmailEvents,
		ResetDemoInventory:       cfg.ResetDemoInventory,
		GeneralTicketID:          cfg.GeneralTicketID,
		VIPTicketID:              cfg.VIPTicketID,
		GeneralPrice:             cfg.GeneralPrice,
		VIPPrice:                 cfg.VIPPrice,
		GeneralStock:             cfg.GeneralStock,
		VIPStock:                 cfg.VIPStock,
		PaymentAPIKey:            "[redacted]",
		CheckInAPIKey:            "[redacted]",
		EmailWebhookSecret:       "[redacted]",
		RateLimitSleepSeconds:    cfg.RateLimitSleepSeconds,
		RequestTimeoutSeconds:    cfg.RequestTimeoutSeconds,
		PaymentWaitAttempts:      cfg.PaymentWaitAttempts,
		IssuedTicketWaitAttempts: cfg.IssuedTicketWaitAttempts,
		RunID:                    cfg.RunID,
		Seed:                     cfg.Seed,
	}
}
