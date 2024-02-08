package worker

import (
	"testing"
	"time"
)

func TestSignalTimeoutDefault(t *testing.T) {
	tests := []struct {
		name        string
		worker      *WorkerConfig
		wantTimeout time.Duration
	}{
		{
			name: "default for zero timeout",
			worker: &WorkerConfig{
				SignalTimeout: 0 * time.Second,
			},
			wantTimeout: DefaultSignalTimeoutSec * time.Second,
		},
		{
			name:        "default for nil worker",
			worker:      nil,
			wantTimeout: DefaultSignalTimeoutSec * time.Second,
		},
		{
			name: "non default for positive timeout",
			worker: &WorkerConfig{
				SignalTimeout: 10 * time.Second,
			},
			wantTimeout: 10 * time.Second,
		},
	}

	for _, tt := range tests {
		if got, want := tt.worker.SignalTimeoutOrDefault(), tt.wantTimeout; got != want {
			t.Errorf("SignalTimeoutOrDefault() case %s = %v, want %v", tt.name, got, tt.wantTimeout)
		}
	}
}

func TestMaxRestartsDefault(t *testing.T) {
	tests := []struct {
		name            string
		worker          *WorkerConfig
		wantMaxRestarts int
	}{
		{
			name: "default for negative max-restarts",
			worker: &WorkerConfig{
				MaxRestarts: -1,
			},
			wantMaxRestarts: DefaultMaxRestarts,
		},
		{
			name: "zero for zero max-restarts",
			worker: &WorkerConfig{
				MaxRestarts: 0,
			},
			wantMaxRestarts: 0,
		},
		{
			name:            "default for nil worker",
			worker:          nil,
			wantMaxRestarts: DefaultMaxRestarts,
		},
		{
			name: "non default for positive max-restarts",
			worker: &WorkerConfig{
				MaxRestarts: 10,
			},
			wantMaxRestarts: 10,
		},
	}

	for _, tt := range tests {
		if got, want := tt.worker.MaxRestartsOrDefault(), tt.wantMaxRestarts; got != want {
			t.Errorf("SignalTimeoutOrDefault() case %s = %v, want %v", tt.name, got, tt.wantMaxRestarts)
		}
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name           string
		worker         *WorkerConfig
		shouldValidate bool
	}{
		{
			name: "Should Validate",
			worker: &WorkerConfig{
				Name:        "foo",
				Replicas:    5,
				Exec:        "bar",
				Args:        []string{"--switch"},
				MaxRestarts: 5,
				Limits:      nil,
			},
			shouldValidate: true,
		},
		{
			name: "Should not Validate with no Name",
			worker: &WorkerConfig{
				Replicas:    5,
				Exec:        "bar",
				Args:        []string{"--switch"},
				MaxRestarts: 5,
				Limits:      nil,
			},
			shouldValidate: false,
		},
		{
			name: "Should not Validate with no Replicas",
			worker: &WorkerConfig{
				Name:        "foo",
				Replicas:    0,
				Exec:        "bar",
				Args:        []string{"--switch"},
				MaxRestarts: 5,
				Limits:      nil,
			},
			shouldValidate: false,
		},
		{
			name: "Should not Validate with no Exec",
			worker: &WorkerConfig{
				Name:        "foo",
				Replicas:    5,
				Args:        []string{"--switch"},
				MaxRestarts: 5,
				Limits:      nil,
			},
			shouldValidate: false,
		},
	}

	for _, tt := range tests {
		err := tt.worker.IsValid()
		if got, want := err == nil, tt.shouldValidate; got != want {
			t.Errorf("IsValid() case %s = %v, want %v error: %v", tt.name, got, want, err)
		}
	}
}
