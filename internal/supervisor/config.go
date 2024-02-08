package supervisor

import "github.com/reddit/monoceros/internal/worker"

type SupervisorConfig struct {
	Workers []worker.WorkerConfig `yaml:"workers"`
}

func (s *SupervisorConfig) IsValid() error {
	for _, w := range s.Workers {
		if err := w.IsValid(); err != nil {
			return err
		}
	}

	return nil
}
