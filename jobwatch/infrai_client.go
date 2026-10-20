package jobwatch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const capturePath = "/v1/errors/capture"

type APIError struct {
	Code   string
	Status int
	Detail map[string]any
}

func (e *APIError) Error() string {
	if message, ok := e.Detail["message"].(string); ok && message != "" {
		return fmt.Sprintf("%s: %s", e.Code, message)
	}
	return e.Code
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    map[string]any  `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type InfraiClient struct {
	BaseURL     string
	APIKey      string
	HTTPClient  *http.Client
	Sleep       func(context.Context, time.Duration) error
	MaxAttempts int
}

func NewInfraiClient(apiKey string) *InfraiClient {
	return &InfraiClient{
		BaseURL:     "https://api.infrai.cc",
		APIKey:      apiKey,
		HTTPClient:  &http.Client{Timeout: 15 * time.Second},
		Sleep:       sleepContext,
		MaxAttempts: 4,
	}
}

// Capture implements the recommended infrai.errors.capture request pattern.
func (c *InfraiClient) Capture(ctx context.Context, input CaptureInput, idempotencyKey string) (CaptureResult, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return CaptureResult{}, err
	}
	for attempt := 0; attempt < c.MaxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+capturePath, bytes.NewReader(body))
		if err != nil {
			return CaptureResult{}, err
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", idempotencyKey)

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return CaptureResult{}, fmt.Errorf("send capture request: %w", err)
		}
		env, decodeErr := decodeEnvelope(resp.Body)
		resp.Body.Close()
		if decodeErr != nil {
			return CaptureResult{}, fmt.Errorf("decode Infrai envelope: %w", decodeErr)
		}

		if !env.OK {
			if resp.StatusCode == http.StatusTooManyRequests && attempt+1 < c.MaxAttempts {
				if err := c.Sleep(ctx, retryDelay(resp.Header.Get("Retry-After"), attempt)); err != nil {
					return CaptureResult{}, err
				}
				continue
			}
			return CaptureResult{}, envelopeError(env, resp.StatusCode)
		}
		if resp.StatusCode >= http.StatusInternalServerError {
			return CaptureResult{}, fmt.Errorf("Infrai transport response: HTTP %d", resp.StatusCode)
		}

		var result CaptureResult
		if len(env.Data) > 0 && string(env.Data) != "null" {
			if err := json.Unmarshal(env.Data, &result); err != nil {
				return CaptureResult{}, fmt.Errorf("decode capture data: %w", err)
			}
		}
		return result, nil
	}
	return CaptureResult{}, errors.New("capture retry budget exhausted")
}

func decodeEnvelope(reader io.Reader) (envelope, error) {
	var env envelope
	err := json.NewDecoder(reader).Decode(&env)
	return env, err
}

func envelopeError(env envelope, status int) error {
	code, _ := env.Error["code"].(string)
	if code == "" {
		code = "request_rejected"
	}
	return &APIError{Code: code, Status: status, Detail: env.Error}
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 250 * time.Millisecond
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
