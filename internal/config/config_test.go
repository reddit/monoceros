package config

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/reddit/monoceros/internal/admin"
	"github.com/reddit/monoceros/internal/supervisor"
	"github.com/reddit/monoceros/internal/worker"
)

func TestLoadConf(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		wantConf MonocerosConfig
	}{
		{
			name: "basic",
			file: "./data/basic.yaml",
			wantConf: MonocerosConfig{
				AdminConfig: admin.AdminConfig{
					Port: 0,
				},
				SupervisorConfig: supervisor.SupervisorConfig{
					Workers: []worker.WorkerConfig{
						{
							Name:          "processbin",
							Exec:          "./dist/processbin",
							Args:          []string{"--exit-after", "10"},
							MaxRestarts:   0,
							Replicas:      1,
							SignalTimeout: 15 * time.Second,
						},
					},
				},
			},
		},
		{
			name: "basic with limits",
			file: "./data/basic_with_limits.yaml",
			wantConf: MonocerosConfig{
				AdminConfig: admin.AdminConfig{
					Port: 0,
				},
				SupervisorConfig: supervisor.SupervisorConfig{
					Workers: []worker.WorkerConfig{
						{
							Name:          "processbin",
							Exec:          "./dist/processbin",
							Args:          []string{"--exit-after", "10"},
							MaxRestarts:   0,
							Replicas:      1,
							SignalTimeout: 15 * time.Second,
							Limits: &worker.LimitsConfig{
								Lifespan: 2 * time.Second,
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		c, err := LoadConf(tt.file)
		if err != nil {
			t.Fatalf("%s-%s: unexpected error when calling LoadConf err= %t", t.Name(), tt.name, err)
		}
		if diff := cmp.Diff(*c, tt.wantConf); diff != "" {
			t.Errorf(
				"%s-%s:: (-got +want)\n%s", t.Name(), tt.name, diff,
			)
		}
	}
}
