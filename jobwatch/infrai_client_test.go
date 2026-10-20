package jobwatch

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestCaptureDecodesRejectionBeforeStatus(t *testing.T) {
	client := NewInfraiClient("test-key")
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost || req.URL.Path != capturePath {
			t.Fatalf("request = %s %s", req.Method, req.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"ok":false,"data":null,"error":{"code":"BAD_EVENT","message":"event rejected"},"metadata":{}}`)),
		}, nil
	})}

	_, err := client.Capture(context.Background(), CaptureInput{}, "event-1")
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Code != "BAD_EVENT" || apiErr.Status != http.StatusBadRequest {
		t.Fatalf("Capture() error = %#v", err)
	}
}

func TestCaptureRetriesRateLimitWithSameKey(t *testing.T) {
	requests := 0
	keys := make([]string, 0, 2)
	client := NewInfraiClient("test-key")
	client.Sleep = func(_ context.Context, delay time.Duration) error {
		if delay != 2*time.Second {
			t.Fatalf("delay = %v", delay)
		}
		return nil
	}
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		keys = append(keys, req.Header.Get("Idempotency-Key"))
		if requests == 1 {
			return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"2"}}, Body: io.NopCloser(strings.NewReader(`{"ok":false,"error":{"code":"RATE_LIMITED"},"metadata":{}}`))}, nil
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true,"data":{"event_id":"evt_9","error_group_id":"grp_3"},"error":null,"metadata":{}}`))}, nil
	})}

	result, err := client.Capture(context.Background(), CaptureInput{}, "job-run-9")
	if err != nil || result.EventID != "evt_9" {
		t.Fatalf("Capture() = %+v, %v", result, err)
	}
	if requests != 2 || keys[0] != "job-run-9" || keys[1] != "job-run-9" {
		t.Fatalf("requests = %d, keys = %v", requests, keys)
	}
}
