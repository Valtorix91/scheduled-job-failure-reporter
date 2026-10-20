package jobwatch

import (
	"context"
	"reflect"
	"testing"
)

type captureSpy struct {
	calls int
	input CaptureInput
	key   string
}

func (s *captureSpy) Capture(_ context.Context, input CaptureInput, key string) (CaptureResult, error) {
	s.calls++
	s.input = input
	s.key = key
	return CaptureResult{EventID: "evt_42", ErrorGroupID: "grp_build"}, nil
}

func TestReporterDecision(t *testing.T) {
	tests := []struct {
		name          string
		event         BuildEvent
		wantPublished bool
		wantCalls     int
		wantErr       bool
	}{
		{
			name:  "successful release stays quiet",
			event: BuildEvent{Job: "release-cli", RunID: "831", Revision: "a19c", Operation: "publish", Status: "succeeded"},
		},
		{
			name:          "failed release is captured",
			event:         BuildEvent{Job: "release-cli", RunID: "832", Revision: "b20d", Operation: "publish", Status: "failed", Detail: "artifact checksum mismatch"},
			wantPublished: true,
			wantCalls:     1,
		},
		{
			name:    "unfinished status is rejected",
			event:   BuildEvent{Job: "release-cli", RunID: "833", Revision: "c31e", Operation: "publish", Status: "running"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spy := &captureSpy{}
			out, err := (Reporter{Capturer: spy}).Record(context.Background(), tt.event)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Record() error = %v, wantErr %v", err, tt.wantErr)
			}
			if out.Published != tt.wantPublished || spy.calls != tt.wantCalls {
				t.Fatalf("Record() = %+v, capture calls = %d", out, spy.calls)
			}
			if tt.wantPublished {
				wantFingerprint := []string{"scheduled-job", "release-cli", "publish"}
				if !reflect.DeepEqual(spy.input.Fingerprint, wantFingerprint) {
					t.Fatalf("fingerprint = %v, want %v", spy.input.Fingerprint, wantFingerprint)
				}
				if spy.key != "job-run-832" {
					t.Fatalf("idempotency key = %q", spy.key)
				}
			}
		})
	}
}
