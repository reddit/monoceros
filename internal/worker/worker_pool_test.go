package worker

import (
	"errors"
	"fmt"
	"io"
	"math"
	"path"
	"reflect"
	"testing"
	"time"

	"github.com/go-cmd/cmd"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/reddit/monoceros/internal/common"
	"github.com/reddit/monoceros/internal/server"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestWorkerPoolNew(t *testing.T) {
	tests := []struct {
		name      string
		execName  string
		logger    *zap.Logger
		replicas  int
		wantError error
	}{
		{
			name:      "Should construct on valid argument",
			execName:  processBinPath,
			logger:    zaptest.NewLogger(t),
			replicas:  1,
			wantError: nil,
		},
		{
			name:      "Should return error on invalid path",
			execName:  "this is not a valid path to a process",
			logger:    zaptest.NewLogger(t),
			replicas:  1,
			wantError: errExecutableNotFound,
		},
		{
			name:      "Should return error on null logger",
			execName:  processBinPath,
			logger:    nil,
			replicas:  1,
			wantError: common.ErrRequirementParameterMissing,
		},
		{
			name:      "Should return error on 0 replicas",
			execName:  processBinPath,
			logger:    zaptest.NewLogger(t),
			replicas:  0,
			wantError: common.ErrRequirementParameterInvalid,
		},
	}

	for _, tt := range tests {
		c := WorkerConfig{
			Name:          "foo",
			Exec:          tt.execName,
			Args:          []string{},
			Replicas:      tt.replicas,
			SignalTimeout: 42 * time.Second,
			MaxRestarts:   0,
		}
		ch := make(chan SupervisorEvent)
		_, errGot := NewWorkerPool(c, ch, tt.logger, EmptyMemoryWorker)
		if !errors.Is(errGot, tt.wantError) {
			t.Errorf("%s: failed = %t, want %t", tt.name, errGot, tt.wantError)
		}
	}
}
func MakePool(c WorkerConfig, ch chan SupervisorEvent, f NewWorkerFunc, t *testing.T) *WorkerPool {
	p, err := NewWorkerPool(c, ch, zaptest.NewLogger(t), f)
	if err != nil {
		t.Fatalf("%s: unexpected error when calling NewWorkerPool err= %t", t.Name(), err)
	}

	if p.HasProxy() {
		// Override the tests to use stdout to generate the config
		p.sInfo.envoyGen.wr = io.Discard
	}

	err = p.PrepareWorkers(cmd.Options{})
	if err != nil {
		t.Fatalf("%s: unexpected error when calling PrepareWorkers err= %t", t.Name(), err)
	}

	return p
}

func MakeAndRunPool(c WorkerConfig, ch chan SupervisorEvent, f NewWorkerFunc, t *testing.T) *WorkerPool {
	p := MakePool(c, ch, f, t)
	p.RunAll()

	workersCount := c.Replicas
	proxyCount := 0
	if p.HasProxy() {
		proxyCount = 1
	}

	if got, want := len(p.workers), (workersCount + proxyCount); got != want {
		t.Errorf("Worker pool created worker replicas= %v, want %v", got, want)
	}

	for i, w := range p.workers {
		if got, want := w.Status(), Running; got != want {
			t.Errorf("Worker %s  state= %v, want %v", i, got, want)
		}
	}
	return p
}

func InterruptPool(p *WorkerPool, ch chan SupervisorEvent, t *testing.T) {
	p.InterruptAll()
	for _, w := range p.workers {
		status := <-ch
		if got, want := status.EventType, ProcessEnd; got != want {
			t.Fatalf("Validating that processes ended test %s =  got event %d, want %d", t.Name(), got, want)
		}
		err := w.WaitForWorkerToCleanBuffer()
		if err != nil {
			t.Errorf("%s unexpected error %v while waiting for stdstream buff",
				t.Name(), err)
		}
	}
}

func TestWorkerPoolPrepareWorkersAllReplicas(t *testing.T) {
	ch := make(chan SupervisorEvent)
	c := WorkerConfig{
		Name:          "foo",
		Exec:          processBinPath,
		Args:          []string{},
		Replicas:      5,
		SignalTimeout: 42,
		MaxRestarts:   0,
	}
	p := MakeAndRunPool(c, ch, LongRunningMemoryWorker, t)
	InterruptPool(p, ch, t)
}

func TestProxyModePool(t *testing.T) {
	t.Run("Should prepare proxy with correct worker config", func(t *testing.T) {
		c := WorkerConfig{
			Name:          "foo",
			Exec:          processBinPath,
			Args:          []string{},
			Replicas:      5,
			SignalTimeout: 42,
			MaxRestarts:   0,
			Server: &server.ServerConfig{
				PortArg: server.PortArgConfig{
					Name:          "bind",
					StartPort:     uint64(900),
					ValueTemplate: "{{.Port}}",
				},
			},
		}

		f := func(wconf WorkerIntConfig) Worker {
			wk := ShortRunningMemoryWorker(wconf)
			if wconf.name == ProxyId {
				if got, want := wconf.collectProcessMetrics, false; got != want {
					t.Errorf("%s: Proxy expected to not collectProcessMetrics", t.Name())
				}

				if got, want := wconf.execName, envoyExec; got != want {
					t.Errorf("%s: Proxy expected to have name got %s want %s", t.Name(), got, want)
				}

				configDefaultPath := path.Join("envoy", envoyBootstrapConfig)
				if got, want := wconf.args, []string{
					"--drain-time-s", fmt.Sprint(math.Ceil(c.SignalTimeoutOrDefault().Seconds())),
					"--drain-strategy", "immediate",
					"-c", configDefaultPath}; !reflect.DeepEqual(got, want) {
					t.Errorf("%s: Proxy expected to have args got %v want %v", t.Name(), got, want)
				}

				if wconf.lifespan != nil {
					t.Errorf("%s: Proxy expected to not have lifespan", t.Name())
				}

				if wconf.memory != nil {
					t.Errorf("%s: Proxy expected to not have memory", t.Name())
				}

			}
			return wk
		}

		_ = MakePool(c, make(chan SupervisorEvent), f, t)
	})

	t.Run("Should set a different port on different workers", func(t *testing.T) {
		ch := make(chan SupervisorEvent)
		startPort := uint64(900)
		portArgName := "bind"
		c := WorkerConfig{
			Name:          "foo",
			Exec:          processBinPath,
			Args:          []string{},
			Replicas:      5,
			SignalTimeout: 42,
			MaxRestarts:   0,
			Server: &server.ServerConfig{
				PortArg: server.PortArgConfig{
					Name:          portArgName,
					StartPort:     startPort,
					ValueTemplate: "{{.Port}}",
				},
			},
		}

		counter := 0
		f := func(wconf WorkerIntConfig) Worker {
			wk := ShortRunningMemoryWorker(wconf)
			if wconf.name != ProxyId {
				if got, want := wconf.args[0], portArgName; got != want {
					t.Fatalf("Test %s =  got first arg %s, want %s", t.Name(), got, want)
				}

				if got, want := wconf.args[1], fmt.Sprint(startPort+uint64(counter)); got != want {
					t.Fatalf("Test %s =  got second arg %s, want %d", t.Name(), got, startPort+uint64(counter))
				}

				counter += 1
			}
			return wk
		}

		_ = MakePool(c, ch, f, t)
	})
}

func TestWorkerPoolRestartWorkers(t *testing.T) {
	restartsCounter.Reset()
	opt := cmd.Options{}
	ch := make(chan SupervisorEvent)
	maxRestarts := 1
	c := WorkerConfig{
		Name:          "foo",
		Exec:          processBinPath,
		Args:          []string{},
		Replicas:      1,
		SignalTimeout: 42 * time.Second,
		MaxRestarts:   maxRestarts,
	}
	p := MakeAndRunPool(c, ch, ShortRunningMemoryWorker, t)
	status := <-ch
	if got, want := status.EventType, ProcessEnd; got != want {
		t.Fatalf("Validating that processes ended test %s =  got event %d, want %d", t.Name(), got, want)
	}

	err := p.RestartWorker(opt, status.Id)
	if err != nil {
		t.Fatalf("%s: unexpected error when calling RestartWorker err= %t", t.Name(), err)
	}
	// wait for process to end and attempt to restart again
	status = <-ch
	if got, want := status.EventType, ProcessEnd; got != want {
		t.Fatalf("Validating that processes ended test %s =  got event %d, want %d", t.Name(), got, want)
	}

	errGot, errWant := p.RestartWorker(opt, status.Id), ErrTooManyRestarts
	if !errors.Is(errGot, errWant) {
		t.Errorf("%s: failed = %t, want %t", t.Name(), errGot, errWant)
	}
	InterruptPool(p, ch, t)

	if got, want := testutil.CollectAndCount(restartsCounter), 2; got != want {
		t.Errorf("%s. Expected restartsCounter counter to track %v, want %v", t.Name(), got, want)
	}

	if got, want :=
		testutil.ToFloat64(restartsCounter.WithLabelValues(p.poolName, "worker")), float64(maxRestarts); got != want {
		t.Errorf("%s. Expected restartsCounter counter got %v, want %v", t.Name(), got, want)
	}
}

func TestWorkerEnforcesLimits(t *testing.T) {
	ch := make(chan SupervisorEvent)
	c := WorkerConfig{
		Name:          "foo",
		Exec:          processBinPath,
		Args:          []string{},
		Replicas:      1,
		SignalTimeout: 42,
		MaxRestarts:   0,
		Limits: &LimitsConfig{
			Lifespan: 1 * time.Second,
		},
	}
	p := MakeAndRunPool(c, ch, LongRunningMemoryWorker, t)
	status := <-ch
	if got, want := status.EventType, ProcessExceededLimit; got != want {
		t.Fatalf("Validating that processes exceeded limit %s =  got event %d, want %d", t.Name(), got, want)
	}
	InterruptPool(p, ch, t)
}

func TestWorkerSigKillCounters(t *testing.T) {
	sigkillCounter.Reset()
	defer sigkillCounter.Reset()
	ch := make(chan SupervisorEvent)
	c := WorkerConfig{
		Name:          "foo",
		Exec:          processBinPath,
		Args:          []string{},
		Replicas:      1,
		SignalTimeout: 42,
		MaxRestarts:   0,
	}
	p := MakeAndRunPool(c, ch, IgnoresInterruptMemoryWorker, t)

	for id := range p.workers {
		p.ForceTerminateWorker(id)
	}

	if got, want := testutil.CollectAndCount(sigkillCounter), len(p.workers); got != want {
		t.Errorf("%s. Expected SIGKILL counter got %v`, want %v", t.Name(), got, want)
	}
}

func TestWorkerPool_IsHealthy(t *testing.T) {
	healthyWorker := &InMemoryWorker{status: Running}
	unhealthyWorker := &InMemoryWorker{status: Terminated}

	tests := []struct {
		name             string
		workers          map[string]Worker
		callRun          map[string]bool
		healthPercentage int
		expectHealthy    bool
	}{
		{
			name: "all workers healthy with 100%",
			workers: map[string]Worker{
				"1": healthyWorker,
				"2": healthyWorker,
				"3": healthyWorker,
				"4": healthyWorker,
			},
			healthPercentage: 100,
			expectHealthy:    true,
		},
		{
			name: "1 worker unhealthy with 100%",
			workers: map[string]Worker{
				"1": healthyWorker,
				"2": healthyWorker,
				"3": unhealthyWorker,
				"4": healthyWorker,
			},
			healthPercentage: 100,
			expectHealthy:    false,
		},
		{
			name: "all workers unhealthy with 100%",
			workers: map[string]Worker{
				"1": unhealthyWorker,
				"2": unhealthyWorker,
				"3": unhealthyWorker,
				"4": unhealthyWorker,
			},
			healthPercentage: 100,
			expectHealthy:    false,
		},
		{
			name: "all workers healthy with 75%",
			workers: map[string]Worker{
				"1": healthyWorker,
				"2": healthyWorker,
				"3": healthyWorker,
				"4": healthyWorker,
			},
			healthPercentage: 75,
			expectHealthy:    true,
		},
		{
			name: "1 worker unhealthy with 75%",
			workers: map[string]Worker{
				"1": healthyWorker,
				"2": healthyWorker,
				"3": unhealthyWorker,
				"4": healthyWorker,
			},
			healthPercentage: 75,
			expectHealthy:    true,
		},
		{
			name: "all workers unhealthy with 75%",
			workers: map[string]Worker{
				"1": unhealthyWorker,
				"2": unhealthyWorker,
				"3": unhealthyWorker,
				"4": unhealthyWorker,
			},
			healthPercentage: 75,
			expectHealthy:    false,
		},
		{
			name: "1 worker unhealthy with 76%",
			workers: map[string]Worker{
				"1": healthyWorker,
				"2": unhealthyWorker,
				"3": healthyWorker,
				"4": healthyWorker,
			},
			healthPercentage: 76,
			expectHealthy:    false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {

			pool := WorkerPool{
				workers:           test.workers,
				replicas:          len(test.workers),
				desiredHealthPcnt: test.healthPercentage,
			}
			if pool.IsHealthy() != test.expectHealthy {
				t.Errorf("expected pool health: %t", test.expectHealthy)
			}
		})
	}
}
