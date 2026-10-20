package jobwatch

import (
	"context"
	"fmt"
)

type BuildEvent struct {
	Job       string `json:"job"`
	RunID     string `json:"run_id"`
	Revision  string `json:"revision"`
	Operation string `json:"operation"`
	Status    string `json:"status"`
	Detail    string `json:"detail"`
}

type CaptureInput struct {
	Message     string         `json:"message"`
	Level       string         `json:"level"`
	Fingerprint []string       `json:"fingerprint"`
	Exception   string         `json:"exception"`
	Context     map[string]any `json:"context"`
}

type CaptureResult struct {
	EventID      string `json:"event_id,omitempty"`
	ErrorGroupID string `json:"error_group_id,omitempty"`
}

type Capturer interface {
	Capture(context.Context, CaptureInput, string) (CaptureResult, error)
}

type Outcome struct {
	Published    bool   `json:"published"`
	EventID      string `json:"event_id,omitempty"`
	ErrorGroupID string `json:"error_group_id,omitempty"`
}

type Reporter struct {
	Capturer Capturer
}

type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

func (r Reporter) Record(ctx context.Context, event BuildEvent) (Outcome, error) {
	if err := validate(event); err != nil {
		return Outcome{}, err
	}
	if event.Status == "succeeded" {
		return Outcome{Published: false}, nil
	}

	input := CaptureInput{
		Message:     fmt.Sprintf("scheduled job %s failed during %s", event.Job, event.Operation),
		Level:       "error",
		Fingerprint: []string{"scheduled-job", event.Job, event.Operation},
		Exception:   event.Detail,
		Context: map[string]any{
			"job":       event.Job,
			"run_id":    event.RunID,
			"revision":  event.Revision,
			"operation": event.Operation,
		},
	}
	result, err := r.Capturer.Capture(ctx, input, "job-run-"+event.RunID)
	if err != nil {
		return Outcome{}, err
	}
	return Outcome{Published: true, EventID: result.EventID, ErrorGroupID: result.ErrorGroupID}, nil
}

func validate(event BuildEvent) error {
	if event.Job == "" || event.RunID == "" || event.Revision == "" || event.Operation == "" {
		return &ValidationError{Message: "job, run_id, revision, and operation are required"}
	}
	if event.Status != "succeeded" && event.Status != "failed" {
		return &ValidationError{Message: "status must be succeeded or failed"}
	}
	if event.Status == "failed" && event.Detail == "" {
		return &ValidationError{Message: "detail is required for a failed job"}
	}
	return nil
}
