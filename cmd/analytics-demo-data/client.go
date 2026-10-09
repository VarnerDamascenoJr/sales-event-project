package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type apiClient struct {
	baseURL             string
	httpClient          *http.Client
	rateLimitSleep      time.Duration
	rateLimitRetryCount int
}

type apiError struct {
	status int
	body   string
}

func (e apiError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.status, e.body)
}

func (c *apiClient) requestJSON(ctx context.Context, method string, path string, payload any, headers map[string]string, expectedStatuses []int, target any) error {
	var body []byte
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = encoded
	}

	url := c.baseURL + path
	for attempt := 1; ; attempt++ {
		request, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
		if err != nil {
			return err
		}
		request.Header.Set("Accept", "application/json")
		if payload != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		for key, value := range headers {
			request.Header.Set(key, value)
		}

		response, err := c.httpClient.Do(request)
		if err != nil {
			return err
		}
		responseBody, readErr := io.ReadAll(response.Body)
		closeErr := response.Body.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}

		if response.StatusCode == http.StatusTooManyRequests && attempt <= 5 {
			c.rateLimitRetryCount++
			if !sleep(ctx, c.rateLimitSleep*time.Duration(attempt)) {
				return ctx.Err()
			}
			continue
		}
		if !statusAllowed(response.StatusCode, expectedStatuses) {
			return apiError{status: response.StatusCode, body: string(responseBody)}
		}
		if target != nil && len(responseBody) > 0 {
			if err := json.Unmarshal(responseBody, target); err != nil {
				return err
			}
		}
		return nil
	}
}

func statusAllowed(status int, expected []int) bool {
	for _, candidate := range expected {
		if status == candidate {
			return true
		}
	}
	return false
}

func sleep(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
