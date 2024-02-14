package supervisor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/reddit/monoceros/metrics"

	"github.com/reddit/monoceros/internal/common"
	"github.com/reddit/monoceros/internal/server"
	"github.com/reddit/monoceros/internal/worker"
	"go.uber.org/zap/zaptest"
)

var processBinPath = "../../dist/processbin"

func TestNewSupervisor(t *testing.T) {
	sConfig := SupervisorConfig{
		Workers: []worker.WorkerConfig{
			{
				Name:     "foo",
				Exec:     processBinPath,
				Replicas: 1,
			},
			{
				Name:     "foo",
				Exec:     processBinPath,
				Replicas: 1,
			},
		},
	}
	_, errGot := NewSupervisor(sConfig, worker.EmptyMemoryWorker, nil)
	if !errors.Is(errGot, common.ErrRequirementParameterMissing) {
		t.Errorf("%s: failed = %t, want %t", t.Name(), errGot, common.ErrRequirementParameterMissing)
	}
}

func TestSupervisorReturnsErrorOnCollisions(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	sConfig := SupervisorConfig{
		Workers: []worker.WorkerConfig{
			{
				Name:     "foo",
				Exec:     processBinPath,
				Replicas: 1,
			},
			{
				Name:     "foo",
				Exec:     processBinPath,
				Replicas: 1,
			},
		},
	}

	s, err := NewSupervisor(sConfig, worker.EmptyMemoryWorker, zaptest.NewLogger(t))
	if err != nil {
		t.Fatalf("%s: unexpected error when calling NewSupervisor err= %t", t.Name(), err)
	}
	errGot := s.Run(ctx, cancel)
	if !errors.Is(errGot, errDuplicateWorkerName) {
		t.Errorf("%s: failed = %t, want %t", t.Name(), errGot, errDuplicateWorkerName)
	}
}

// TODO: Issue #69 part 2
//
//nolint:gocyclo
func TestSupervisorRun(t *testing.T) {
	t.Run("Should restart workers until max restarts", func(t *testing.T) {
		defer metrics.ResetWorkerGauge()
		maxRestarts := 2
		tests := []struct {
			name          string
			newWorkerFunc worker.NewWorkerFunc
			c             SupervisorConfig
			integration   bool
		}{
			{
				name:          "using fake worker",
				newWorkerFunc: worker.ShortRunningMemoryWorker,
				c: SupervisorConfig{[]worker.WorkerConfig{
					{
						Name:        "foo",
						Exec:        processBinPath,
						MaxRestarts: maxRestarts,
						Replicas:    3,
					},
				},
				},
			},
			{
				name:          "using real worker",
				newWorkerFunc: worker.NewProdWorker,
				integration:   true,
				c: SupervisorConfig{[]worker.WorkerConfig{
					{
						Name:        "processbin",
						Exec:        processBinPath,
						Args:        []string{"--exit-after", "1"},
						MaxRestarts: maxRestarts,
						Replicas:    3,
					},
				},
				},
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				// This is fairly ugly, but it needs to happen as test cleanup
				defer metrics.ResetWorkerGauge()

				if test.integration && testing.Short() {
					t.Skip("skipping integration test")
				}

				ctx := context.Background()
				ctx, cancel := context.WithCancel(ctx)
				s, err := NewSupervisor(test.c, test.newWorkerFunc, zaptest.NewLogger(t))
				if err != nil {
					t.Fatalf("%s with %s: unexpected error when calling NewSupervisor err= %t", t.Name(), test.name, err)
				}
				err = s.Run(ctx, cancel)

				if err != nil {
					t.Fatalf("%s with %s: unexpected error when calling Run err= %t", t.Name(), test.name, err)
				}

				errGot := ctx.Err()
				if !errors.Is(errGot, context.Canceled) {
					t.Errorf("%s with %s: failed = %t, want %t", t.Name(), test.name, errGot, errDuplicateWorkerName)
				}

				// The pool name just so happens to match worker name
				metrics.ValidateSupervisorEndsStats(t, test.c.Workers[0].Name, test.c.Workers[0].Replicas)
			})
		}
	})

	t.Run("Should restart when limits are hit", func(t *testing.T) {
		defer metrics.ResetWorkerGauge()

		maxRestarts := 2
		tests := []struct {
			name          string
			newWorkerFunc worker.NewWorkerFunc
			c             SupervisorConfig
			integration   bool
		}{
			{
				name:          "using fake worker",
				newWorkerFunc: worker.ShortRunningMemoryWorker,
				c: SupervisorConfig{[]worker.WorkerConfig{
					{
						Name:        "foo",
						Exec:        processBinPath,
						MaxRestarts: maxRestarts,
						Replicas:    3,
						Limits:      &worker.LimitsConfig{Lifespan: 1 * time.Second},
					},
				},
				},
			},
			{
				name:          "using real worker",
				newWorkerFunc: worker.NewProdWorker,
				integration:   true,
				c: SupervisorConfig{[]worker.WorkerConfig{
					{
						Name:          "processbin",
						Exec:          processBinPath,
						Args:          []string{"--exit-after", "200"},
						MaxRestarts:   0,
						Replicas:      3,
						SignalTimeout: 15 * time.Second,
						Limits:        &worker.LimitsConfig{Lifespan: 1 * time.Second},
					},
				},
				},
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.integration && testing.Short() {
					t.Skip("skipping integration test")
				}

				ctx := context.Background()
				ctx, cancel := context.WithCancel(ctx)
				s, err := NewSupervisor(test.c, test.newWorkerFunc, zaptest.NewLogger(t))
				if err != nil {
					t.Fatalf("%s with %s: unexpected error when calling NewSupervisor err= %t", t.Name(), test.name, err)
				}

				err = s.Run(ctx, cancel)
				if err != nil {
					t.Fatalf("%s with %s: unexpected error when calling Run err= %t", t.Name(), test.name, err)
				}

				errGot := ctx.Err()
				if !errors.Is(errGot, context.Canceled) {
					t.Errorf("%s with %s: failed = %t, want %t", t.Name(), test.name, errGot, errDuplicateWorkerName)
				}
			})
		}
	})

	t.Run("Should shutdown all workers when interrupted", func(t *testing.T) {
		defer metrics.ResetWorkerGauge()

		ctx := context.Background()
		ctx, cancel := context.WithCancel(ctx)
		maxRestarts := 100
		sConfig := SupervisorConfig{
			Workers: []worker.WorkerConfig{
				{
					Name:        "foo",
					Exec:        processBinPath,
					MaxRestarts: maxRestarts,
					Replicas:    1,
				},
			},
		}
		s, err := NewSupervisor(sConfig, worker.ShortRunningMemoryWorker, zaptest.NewLogger(t))
		if err != nil {
			t.Fatalf("%s: unexpected error when calling NewSupervisor err= %t", t.Name(), err)
		}

		go func() {
			<-time.After(1 * time.Second)
			cancel()
		}()
		err = s.Run(ctx, cancel)
		if err != nil {
			t.Fatalf("%s: unexpected error when calling Run err= %t", t.Name(), err)
		}

		errGot := ctx.Err()
		if !errors.Is(errGot, context.Canceled) {
			t.Errorf("%s: failed = %t, want %t", t.Name(), errGot, errDuplicateWorkerName)
		}

		metrics.ValidateSupervisorSigtermStats(t)
	})

	t.Run("Should shutdown all workers when interrupt is ignored", func(t *testing.T) {
		defer metrics.ResetWorkerGauge()

		tests := []struct {
			name          string
			newWorkerFunc worker.NewWorkerFunc
			c             SupervisorConfig
			integration   bool
		}{
			{
				name:          "using fake worker",
				newWorkerFunc: worker.IgnoresInterruptMemoryWorker,
				c: SupervisorConfig{[]worker.WorkerConfig{
					{
						Name:          "foo",
						Exec:          processBinPath,
						Replicas:      3,
						SignalTimeout: 1 * time.Second,
					},
				},
				},
			},
			{
				name:          "using real worker",
				newWorkerFunc: worker.NewProdWorker,
				integration:   true,
				c: SupervisorConfig{[]worker.WorkerConfig{
					{
						Name:          "processbin",
						Exec:          processBinPath,
						Args:          []string{"--exit-after", "60", "--ignore-signals"},
						Replicas:      3,
						SignalTimeout: 1 * time.Second,
					},
				},
				},
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.integration && testing.Short() {
					t.Skip("skipping integration test")
				}

				ctx := context.Background()
				ctx, cancel := context.WithCancel(ctx)
				s, err := NewSupervisor(test.c, test.newWorkerFunc, zaptest.NewLogger(t))
				if err != nil {
					t.Fatalf("%s with %s: unexpected error when calling NewSupervisor err= %t", t.Name(), test.name, err)
				}

				go func() {
					<-time.After(1 * time.Second)
					cancel()
				}()

				err = s.Run(ctx, cancel)
				if err != nil {
					t.Fatalf("%s: unexpected error when calling Run err= %t", t.Name(), err)
				}

				errGot := ctx.Err()
				if !errors.Is(errGot, context.Canceled) {
					t.Errorf("%s: failed = %t, want %t", t.Name(), errGot, errDuplicateWorkerName)
				}

				metrics.ValidateSupervisorSigtermStats(t)
			})
		}
	})

	t.Run("All workers start regardless of wait", func(t *testing.T) {
		defer metrics.ResetWorkerGauge()

		tests := []struct {
			name          string
			newWorkerFunc worker.NewWorkerFunc
			c             SupervisorConfig
			integration   bool
		}{
			{
				name:          "using fake worker",
				newWorkerFunc: worker.MediumRunningMemoryWorker,
				c: SupervisorConfig{[]worker.WorkerConfig{
					{
						Name:          "foo",
						Exec:          processBinPath,
						Replicas:      3,
						SignalTimeout: 1 * time.Second,
					},
				},
				},
			},
			{
				name:          "using fake worker with batch",
				newWorkerFunc: worker.MediumRunningMemoryWorker,
				c: SupervisorConfig{[]worker.WorkerConfig{
					{
						Name:            "foo",
						Exec:            processBinPath,
						Replicas:        15,
						SignalTimeout:   1 * time.Second,
						StartupWaitTime: time.Millisecond * 10,
					},
				},
				},
			},
			{
				name:          "using real worker",
				newWorkerFunc: worker.NewProdWorker,
				integration:   true,
				c: SupervisorConfig{[]worker.WorkerConfig{
					{
						Name:          "processbin",
						Exec:          processBinPath,
						Args:          []string{"--exit-after", "60"},
						Replicas:      3,
						SignalTimeout: 1 * time.Second,
					},
				},
				},
			},
			{
				name:          "using real worker with batch",
				newWorkerFunc: worker.NewProdWorker,
				integration:   true,
				c: SupervisorConfig{[]worker.WorkerConfig{
					{
						Name:            "processbin",
						Exec:            processBinPath,
						Args:            []string{"--exit-after", "60"},
						Replicas:        15,
						SignalTimeout:   1 * time.Second,
						StartupWaitTime: time.Millisecond * 10,
					},
				},
				},
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				if test.integration && testing.Short() {
					t.Skip("skipping integration test")
				}

				// This is fairly ugly, but it needs to happen as test cleanup
				defer metrics.ResetWorkerGauge()

				ctx := context.Background()
				ctx, cancel := context.WithCancel(ctx)
				s, err := NewSupervisor(test.c, test.newWorkerFunc, zaptest.NewLogger(t))
				if err != nil {
					t.Fatalf("%s with %s: unexpected error when calling NewSupervisor err= %t", t.Name(), test.name, err)
				}

				go func() {
					<-time.After(time.Second * 1)
					// The pool name just so happens to match worker name
					metrics.ValidateSupervisorStartsStats(t, test.c.Workers[0].Name, test.c.Workers[0].Replicas)
					cancel()
				}()

				err = s.Run(ctx, cancel)
				if err != nil {
					t.Errorf("%s: unexpected error when calling Run err= %t", t.Name(), err)
				}

				errGot := ctx.Err()
				if !errors.Is(errGot, context.Canceled) {
					t.Errorf("%s: failed = %t, want %t", t.Name(), errGot, errDuplicateWorkerName)
				}
			})
		}
	})

	t.Run("Should shutdown all workers when proxy runs", func(t *testing.T) {
		defer metrics.ResetWorkerGauge()
		ctx := context.Background()
		ctx, cancel := context.WithCancel(ctx)
		maxRestarts := 100
		sConfig := SupervisorConfig{
			Workers: []worker.WorkerConfig{
				{
					Name:        "foo",
					Exec:        processBinPath,
					MaxRestarts: maxRestarts,
					Replicas:    1,
					Server: &server.ServerConfig{
						PortArg: server.PortArgConfig{
							Name:          "port",
							StartPort:     9000,
							ValueTemplate: "{{.Port}}",
						},
						HealthCheckPath: "/foo",
					},
				},
			},
		}
		s, err := NewSupervisor(sConfig, worker.ShortRunningMemoryWorker, zaptest.NewLogger(t))
		if err != nil {
			t.Fatalf("%s: unexpected error when calling NewSupervisor err= %t", t.Name(), err)
		}

		go func() {
			<-time.After(1 * time.Second)
			cancel()
		}()
		err = s.Run(ctx, cancel)

		if err != nil {
			t.Fatalf("%s: unexpected error when calling Run err= %t", t.Name(), err)
		}

		errGot := ctx.Err()
		if !errors.Is(errGot, context.Canceled) {
			t.Errorf("%s: failed = %t, want %t", t.Name(), errGot, errDuplicateWorkerName)
		}

		metrics.ValidateSupervisorEndsStats(t, sConfig.Workers[0].Name, sConfig.Workers[0].Replicas+1)
	})

	t.Run("Workers should be reachable via the proxy", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping integration test")
		}

		defer metrics.ResetWorkerGauge()
		ctx := context.Background()
		ctx, cancel := context.WithCancel(ctx)
		maxRestarts := 100
		sConfig := SupervisorConfig{
			Workers: []worker.WorkerConfig{
				{
					Name:        "foo",
					Exec:        processBinPath,
					Args:        []string{"--log-fixed", "1", "--signal-timeout", "5"},
					MaxRestarts: maxRestarts,
					Replicas:    3,
					Server: &server.ServerConfig{
						PortArg: server.PortArgConfig{
							Name:          "--port",
							StartPort:     8000,
							ValueTemplate: "{{.Port}}",
						},
						HealthCheckPath: "/",
						ProxyConfigPath: t.TempDir(),
						ProxyAdminPort:  9001,
					},
				},
			},
		}
		s, err := NewSupervisor(sConfig, worker.NewProdWorker, zaptest.NewLogger(t))
		if err != nil {
			t.Fatalf("%s: unexpected error when calling NewSupervisor err= %t", t.Name(), err)
		}

		supervisorErr := make(chan error, 1)
		go func() {
			supervisorErr <- s.Run(ctx, cancel)
		}()

		client := &http.Client{
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 20,
			},
			Timeout: 10 * time.Second,
		}

		// Poll all workers
		for i := 0; i < sConfig.Workers[0].Replicas; i++ {
			workerStarted := false
			workerPort := sConfig.Workers[0].Server.PortArg.StartPort + uint64(i)
			// Poll until we can start reaching a worker
			for start := time.Now(); time.Since(start) < 5*time.Second; {
				workerRequest, err := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d", workerPort), nil)
				if err != nil {
					t.Fatalf("Error creating request: %v", err)
				}
				resp, err := client.Do(workerRequest)
				if err == nil && resp.StatusCode == 200 {
					defer resp.Body.Close()
					validateProcessbinResponse(resp.Body, t)
					workerStarted = true
					break
				}
			}

			if !workerStarted {
				t.Errorf("%s: worker on port %d should be reachable", t.Name(), workerPort)
			}
		}

		// Poll envoy now
		proxyRequest, err := http.NewRequest("GET", "http://127.0.0.1:10000", nil)
		if err != nil {
			t.Fatalf("Error creating request: %v", err)
		}
		proxyRequest.Header.Set("Connection", "keep-alive")
		proxyRequest.Header.Set("Keep-Alive", "timeout=100, max=1000")

		proxyStarted := false
		for start := time.Now(); time.Since(start) < 15*time.Second && !proxyStarted; {
			resp, err := client.Do(proxyRequest)
			if err == nil && resp.StatusCode == 200 {
				validateProcessbinResponse(resp.Body, t)
				resp.Body.Close()
				proxyStarted = true
				if resp.Close {
					t.Errorf("Proxy should not be closing the connection")
				}
			}
		}

		if !proxyStarted {
			t.Errorf("%s: proxy should be reachable", t.Name())
		}

		cancel()
		// Ensure that envoy shutsdown the connections properly
		connectionClosed := false
		for start := time.Now(); time.Since(start) < 2*time.Second && !connectionClosed; {
			resp, _ := client.Do(proxyRequest)
			if resp.Close {
				connectionClosed = true
			}
			_, err := io.Copy(io.Discard, resp.Body)
			if err != nil {
				t.Errorf("unexpected error %v: ", err)
			}
			resp.Body.Close()
		}

		err = <-supervisorErr
		if !connectionClosed {
			t.Fatalf("Sad")
		}
		if err != nil {
			t.Fatalf("%s: unexpected error when calling Run err= %t", t.Name(), err)
		}

		errGot := ctx.Err()
		if !errors.Is(errGot, context.Canceled) {
			t.Errorf("%s: failed = %t, want %t", t.Name(), errGot, errDuplicateWorkerName)
		}

		metrics.ValidateSupervisorEndsStats(t, sConfig.Workers[0].Name, sConfig.Workers[0].Replicas+1)
	})
}

func TestSupervisor_IsHealthy(t *testing.T) {
	healthyPool := mockWorkerPool{
		isHealthy: func() bool {
			return true
		},
	}
	unhealthyPool := mockWorkerPool{
		isHealthy: func() bool {
			return false
		},
	}

	tests := []struct {
		name          string
		workerPool    map[string]WorkerPoolInfo
		expectHealthy bool
	}{
		{
			name: "1 pool is healthy",
			workerPool: map[string]WorkerPoolInfo{
				"worker1": {pool: healthyPool},
			},
			expectHealthy: true,
		},
		{
			name: "2 pools are healthy",
			workerPool: map[string]WorkerPoolInfo{
				"worker1": {pool: healthyPool},
				"worker2": {pool: healthyPool},
			},
			expectHealthy: true,
		},
		{
			name: "1 pool is unhealthy",
			workerPool: map[string]WorkerPoolInfo{
				"worker1": {pool: unhealthyPool},
			},
			expectHealthy: false,
		},
		{
			name: "some pools healthy some pools not",
			workerPool: map[string]WorkerPoolInfo{
				"worker1": {pool: healthyPool},
				"worker2": {pool: unhealthyPool},
			},
			expectHealthy: false,
		},
		{
			name: "2 pools unhealthy",
			workerPool: map[string]WorkerPoolInfo{
				"worker1": {pool: unhealthyPool},
				"worker2": {pool: unhealthyPool},
			},
			expectHealthy: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			supervisor := Supervisor{
				pools: test.workerPool,
			}
			if supervisor.IsHealthy() != test.expectHealthy {
				t.Errorf("expected IsHealthy value of %t but failed", test.expectHealthy)
			}
		})
	}
}
