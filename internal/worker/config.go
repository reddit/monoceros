package worker

import (
	"fmt"
	"time"

	"github.com/reddit/monoceros/internal/server"
)

type LimitsConfig struct {
	Memory int `yaml:"memory"`
	// Lifespan is the maxium duration the worker should stay
	// alive before the limit is hit
	Lifespan time.Duration `yaml:"lifespan"`
	// JitterRatio is a float number from [0-1] that
	// adds some randomness so that the limiters don't get
	// triggered at the exact same time on all the workers
	JitterRatio float64 `yaml:"jitter_ratio"`
}

type WorkerConfig struct {
	// Name of the worker pool
	Name string `yaml:"name"`
	// How many copies of the worker should be kept alive
	Replicas int `yaml:"replicas"`
	// The path and arguments to the executable
	// Follow the same convention as https://pkg.go.dev/os/exec#Command
	Exec string   `yaml:"exec"`
	Args []string `yaml:"args"`
	// How much time we will wait for the worker to handle SIGTERM
	// after that a SIGKILL is sent to the process
	SignalTimeout time.Duration `yaml:"signal_timeout"`
	// MaxRestarts defines the maximum number of restarts allowed by the workers
	// If a process is crashing aggresively there is no point in restarting it.
	MaxRestarts int `yaml:"max_restarts"`
	// Configuration of the limiters
	Limits *LimitsConfig `yaml:"limits"`
	// Configuration of the server parameters.
	Server *server.ServerConfig `yaml:"server"`
	// How much time should we wait before we consider that a worker has started
	StartupWaitTime time.Duration `yaml:"startup_wait_time"`
	// Percentage of workers that should be started before declaring the pool healthy
	DesiredHealthyPercentage int `yaml:"desired_healthy_percentage"`
}

func (w *WorkerConfig) SignalTimeoutOrDefault() time.Duration {
	if w == nil || w.SignalTimeout == 0 {
		return time.Duration(DefaultSignalTimeoutSec) * time.Second
	}
	return w.SignalTimeout
}

func (w *WorkerConfig) MaxRestartsOrDefault() int {
	if w == nil || w.MaxRestarts < 0 {
		return DefaultMaxRestarts
	}
	return w.MaxRestarts
}

func (w *WorkerConfig) IsValid() error {
	if w.Name == "" {
		return fmt.Errorf("Worker name must be set")
	}

	if w.Replicas <= 0 {
		return fmt.Errorf("Worker replicas can not be less or equal to 0")
	}

	if w.Exec == "" {
		return fmt.Errorf("Worker executable name must be set")
	}

	if err := w.Server.IsValid(); err != nil {
		return err
	}

	return nil
}
