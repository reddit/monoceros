package worker

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/go-cmd/cmd"
	"go.uber.org/zap"
)

// InMemoryWorker is an in-memory worker in the sense
// that it doesn't spawn any external processes.
// It can be used to emulate process that log enormous data, crash, or ignore signals
// This should only be used for tests to make them more hermetic
// since they don't rely on external system calls.
type InMemoryWorker struct {
	Name            string
	PoolName        string
	PRuntime        processRuntimeInfo
	ProcessEventsCh chan SupervisorEvent
	ctx             context.Context
	cancel          context.CancelFunc
	lock            sync.RWMutex
	// Status is updated by multiple coroutines so it needs to be
	// protected by a lock. The rest of the state variables are
	// set when the struct is init and then they are Read only so
	// they dont need locks.
	status ProcessStatus

	// These variables are exposed so we could tinker with the
	// behavior of the worker during testing and inject behavior
	RestartsCounter int
	ExitAfter       time.Duration
	ExitCode        int
	LogText         string
	LogInterval     time.Duration
	LogError        bool
	IgnoreTerm      bool
	Memory          *Limiter
	Lifespan        *Limiter
}

func NewInMemoryWorker(name, pool string,
	ch chan SupervisorEvent,
	e time.Duration,
	c int,
	interval time.Duration) InMemoryWorker {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	return InMemoryWorker{
		Name:            name,
		PoolName:        pool,
		ExitAfter:       e,
		ExitCode:        c,
		LogText:         "log",
		LogInterval:     interval,
		ctx:             ctx,
		cancel:          cancel,
		ProcessEventsCh: ch,
	}
}

func (w *InMemoryWorker) StreamLogs(logger *zap.Logger, outW io.Writer, errW io.Writer) {
	if w.LogInterval == 0 {
		return
	}

	ticker := time.NewTicker(w.LogInterval)
	go func() {
		for {
			<-ticker.C
			if w.Status() == Running {
				LogStdOut(w.Name, w.LogText, logger, outW)
				if w.LogError {
					LogStdErr(w.Name, w.LogText, logger, errW)
				}
			} else {
				return
			}
		}
	}()
}

func (w *InMemoryWorker) Run(_ *zap.Logger) {
	if w.Memory != nil {
		go (*w.Memory).Monitor(w.ProcessEventsCh, w.ctx, nil)
	}

	if w.Lifespan != nil {
		go (*w.Lifespan).Monitor(w.ProcessEventsCh, w.ctx, nil)
	}

	w.PRuntime.startTime = time.Now()
	w.setStatus(Running)
	// Some tests want processes that never terminate on their own
	if w.ExitAfter != 0 {
		go func(t time.Duration) {
			<-time.After(t)
			w.setStatus(Terminated)
			// TODO(monocerosdev): expand this to contain all data
			w.ProcessEventsCh <- SupervisorEvent{
				EventType: ProcessEnd,
				Cmd: &cmd.Status{
					Cmd:      "Dummy name",
					PID:      42,
					Complete: true,
					Exit:     w.ExitCode,
				},
				Id:   w.Name,
				Pool: w.PoolName,
			}
		}(w.ExitAfter)
	}
}

func (w *InMemoryWorker) Terminate(timeout time.Duration) {
	w.cancel()
	if w.IgnoreTerm {
		go func(timeout time.Duration) {
			<-time.After(timeout)
			if w != nil && w.Status() != Terminated {
				w.ProcessEventsCh <- SupervisorEvent{
					EventType: ProcessNeedsTermination,
					Id:        w.Name,
					Pool:      w.PoolName,
				}
			}
		}(timeout)
		w.setStatus(Terminating)
		return
	}
	w.setStatus(Terminated)
	go func() {
		w.ProcessEventsCh <- SupervisorEvent{
			EventType: ProcessEnd,
			Cmd: &cmd.Status{
				Cmd:      "Dummy name",
				PID:      42,
				Complete: true,
				Exit:     w.ExitCode,
			},
			Id:   w.Name,
			Pool: w.PoolName,
		}
	}()
}

func (w *InMemoryWorker) WaitForWorkerToCleanBuffer() error {
	return nil
}

func (w *InMemoryWorker) Status() ProcessStatus {
	w.lock.RLock()
	defer w.lock.RUnlock()
	st := w.status
	return st
}

func (w *InMemoryWorker) setStatus(st ProcessStatus) {
	w.lock.Lock()
	defer w.lock.Unlock()
	w.status = st
}

func (w *InMemoryWorker) Restarts() int {
	return w.RestartsCounter
}

func (w *InMemoryWorker) ForceTerminateWorker() {
	w.setStatus(Terminated)
	go func() {
		w.ProcessEventsCh <- SupervisorEvent{
			EventType: ProcessEnd,
			Cmd: &cmd.Status{
				Cmd:      "Dummy name",
				PID:      42,
				Complete: true,
				Exit:     w.ExitCode,
			},
			Id:   w.Name,
			Pool: w.PoolName,
		}
	}()
}
