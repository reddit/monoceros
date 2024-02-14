package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
	"time"

	"github.com/go-cmd/cmd"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/reddit/monoceros/internal/common"
	"go.uber.org/zap"
)

// realProcessWorker is an abstraction over a system process i.e. worker
// It can be used to manage the lifetime of a process and also
// inject custom behavior such as limiters.
// Workers communicate via events on a channel provided by the
// owner of the worker. Like the supervisor.
type realProcessWorker struct {
	name     string
	poolName string
	runtime  processRuntimeInfo
	channel  chan SupervisorEvent
	restarts int

	// This could be turned into an array but for now
	// we only have two options. so it should be ok
	memory   *Limiter
	lifespan *Limiter
	ctx      context.Context
	cancel   context.CancelFunc

	pidCollector    prometheus.Collector
	isStreamingLogs bool
}

func NewProdWorker(wconf WorkerIntConfig) Worker {
	rv := newWorkerInstance(wconf)
	return &rv
}

func newWorkerInstance(wconf WorkerIntConfig) realProcessWorker {
	wconf.logger.Debug("Worker", zap.String("exec", wconf.execName), zap.Strings("args", wconf.args))
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	command := cmd.NewCmdOptions(wconf.opt, wconf.execName, wconf.args...)

	// Get all the env variables of the current process to pass along and also
	// add MULTIPROCESS_WORKER_ID so the worker has a unique id that it can leverage
	// this is useful as key for multiprocess prometheus clients.
	command.Env = os.Environ()
	command.Env = append(command.Env, fmt.Sprintf("MULTIPROCESS_WORKER_ID=%s_%s", wconf.poolName, wconf.name))
	rv := realProcessWorker{
		runtime: processRuntimeInfo{
			cmd:           command,
			stdStreamChan: make(chan struct{}),
		},
		name:            wconf.name,
		poolName:        wconf.poolName,
		restarts:        wconf.restarts,
		lifespan:        wconf.lifespan,
		memory:          wconf.memory,
		ctx:             ctx,
		cancel:          cancel,
		channel:         wconf.ch,
		isStreamingLogs: wconf.opt.Streaming,
	}

	if wconf.collectProcessMetrics {
		rv.pidCollector = collectors.NewProcessCollector(
			collectors.ProcessCollectorOpts{
				PidFn:        rv.PidFn,
				Namespace:    promNamespace + "_" + subsystem,
				ReportErrors: false,
			},
		)
		unregistered := prometheus.WrapRegistererWith(
			prometheus.Labels{
				poolLabel:   wconf.poolName,
				workerLabel: wconf.name,
			},
			prometheus.DefaultRegisterer,
		).Unregister(rv.pidCollector)
		wconf.logger.Debug("Worker metric unregistered",
			zap.String(common.WorkerIdLogLabel, wconf.name),
			zap.String(common.WorkerPoolLogLabel, wconf.poolName),
			zap.Bool("result", unregistered),
		)
		err := prometheus.WrapRegistererWith(
			prometheus.Labels{
				poolLabel:   wconf.poolName,
				workerLabel: wconf.name,
			},
			prometheus.DefaultRegisterer,
		).Register(rv.pidCollector)
		wconf.logger.Debug("Worker metric registered",
			zap.String(common.WorkerIdLogLabel, wconf.name),
			zap.String(common.WorkerPoolLogLabel, wconf.poolName),
		)
		are := &prometheus.AlreadyRegisteredError{}
		if err != nil {
			if !errors.As(err, are) {
				wconf.logger.Info("Already registered:",
					zap.String(common.WorkerIdLogLabel, wconf.name),
					zap.String(common.WorkerPoolLogLabel, wconf.poolName),
				)
			} else {
				panic(err)
			}
		}
	}

	return rv
}

func (w *realProcessWorker) StreamLogs(logger *zap.Logger, outW io.Writer, errW io.Writer) {
	defer close(w.runtime.stdStreamChan)
	if !w.isStreamingLogs {
		return
	}
	// Done when both channels have been closed
	// https://dave.cheney.net/2013/04/30/curious-channels
	for w.runtime.cmd.Stdout != nil || w.runtime.cmd.Stderr != nil {
		select {
		case line, open := <-w.runtime.cmd.Stdout:
			if !open {
				w.runtime.cmd.Stdout = nil
				continue
			}
			LogStdOut(w.name, line, logger, outW)
		case line, open := <-w.runtime.cmd.Stderr:
			if !open {
				w.runtime.cmd.Stderr = nil
				continue
			}
			LogStdErr(w.name, line, logger, errW)
		}
	}
}

func (w *realProcessWorker) sendUpdatesToAggrChannel(pool string, c <-chan cmd.Status, logger *zap.Logger) {
	defer close(w.channel)
	for msg := range c {
		logger.Info("Worker exited",
			zap.String(common.WorkerIdLogLabel, w.name),
			zap.String(common.WorkerPoolLogLabel, w.poolName),
			zap.Int("rc", w.runtime.cmd.Status().Exit),
			zap.Int("restarts", w.Restarts()))
		w.channel <- SupervisorEvent{
			EventType: ProcessEnd,
			Cmd:       &msg,
			Id:        w.name,
			Pool:      pool,
		}
	}
}

func (w *realProcessWorker) Run(logger *zap.Logger) {
	channel := w.runtime.cmd.Start()
	w.runtime.startTime = time.Now()
	go w.sendUpdatesToAggrChannel(w.poolName, channel, logger)

	if w.memory != nil {
		// TODO(monocerosdev): the memory limiter will be hard if not
		// impossible to test with an in-memory process
		go (*w.memory).Monitor(w.channel, w.ctx, nil)
	}

	if w.lifespan != nil {
		go (*w.lifespan).Monitor(w.channel, w.ctx, nil)
	}
}

func (w *realProcessWorker) Terminate(timeout time.Duration) {
	if w.Status() == Terminated {
		return
	}
	w.cancel()
	err := w.runtime.cmd.Stop()
	// There is always the case that the process hasn't started yet and in that case
	// we can't actually terminate.
	if err == cmd.ErrNotStarted {
		go func() {
			w.channel <- SupervisorEvent{
				EventType:   ProcessNeedsInterrupt,
				EventReason: ProcessHasNotStartedYet,
				Id:          w.name,
				Pool:        w.poolName,
			}
		}()
		// We want to return here to give enough time to the process to handle the interrupt
		// signal and only register the termination timer after it actually stops. Seems better
		// to optimize for graceful shutdown rather than shutting down ASAP.
		return
	}

	go func(timeout time.Duration) {
		<-time.After(timeout)
		if w != nil && w.Status() != Terminated {
			w.channel <- SupervisorEvent{
				EventType: ProcessNeedsTermination,
				Id:        w.name,
				Pool:      w.poolName,
			}
		}
	}(timeout)
}

func (w *realProcessWorker) WaitForWorkerToCleanBuffer() error {
	if w.Status() != Terminated {
		return fmt.Errorf("%w: cannot wait for worker to clean buffer if worker is at state %d ",
			common.ErrInvalidState, w.Status())
	}

	if !w.isStreamingLogs {
		return nil
	}

	<-w.runtime.stdStreamChan
	return nil
}

func (w *realProcessWorker) Status() ProcessStatus {
	if w.runtime.cmd.Status().Complete || w.runtime.cmd.Status().StopTs > 0 {
		return Terminated
	}

	if w.runtime.cmd.Status().PID != 0 {
		return Running
	}

	return PrepareWorkers
}

func (w *realProcessWorker) Restarts() int {
	return w.restarts
}

func (w *realProcessWorker) ForceTerminateWorker() {
	// TODO: We need to handle the error here if it ever happens
	//nolint:errcheck
	syscall.Kill(-w.runtime.cmd.Status().PID, syscall.SIGKILL)
}

func (w *realProcessWorker) PidFn() (int, error) {
	pid := w.runtime.cmd.Status().PID
	if pid == 0 {
		return 0, errors.New("worker not running")
	}
	return pid, nil
}
