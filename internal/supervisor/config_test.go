package supervisor

import (
	"testing"

	"github.com/reddit/monoceros/internal/worker"
)

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name           string
		workers        []worker.WorkerConfig
		shouldValidate bool
	}{
		{
			name: "Should Validate with valid workers",
			workers: []worker.WorkerConfig{
				{
					Name:        "foo",
					Replicas:    5,
					Exec:        "bar",
					Args:        []string{"--switch"},
					MaxRestarts: 5,
					Limits:      nil,
				},
			},
			shouldValidate: true,
		},
		{
			name: "Should Validate with invalid workers",
			workers: []worker.WorkerConfig{
				{},
			},
			shouldValidate: false,
		},
	}

	for _, tt := range tests {
		s := SupervisorConfig{
			Workers: tt.workers,
		}
		err := s.IsValid()
		if got, want := err == nil, tt.shouldValidate; got != want {
			t.Errorf("IsValid() case %s = %v, want %v error: %v", tt.name, got, want, err)
		}
	}

}
