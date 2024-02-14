package worker

import (
	"context"
	"io"
	"time"

	"github.com/go-cmd/cmd"
	"go.uber.org/zap"
)

type ProcessStatus int

const (
	// The process cmd is PrepareWorkers but the process is not started yet.
	PrepareWorkers ProcessStatus = iota
	// The process is expected to be running.
	Running
	// The process has received a SIGINT and is cleaning up.
	Terminating
	// The process is already terminated.
	Terminated
)

type EventType int

const (
	// Event types passed through the SupervisorEvent.
	ProcessEnd EventType = iota
	ProcessNeedsTermination
	ProcessExceededLimit
	ProcessNeedsInterrupt
)

type EventReason int

const (
	// Event type reason.
	ProcessExited EventReason = iota
	ProcessInterruptedBySupervisor
	TimeLimitExceeded
	ProcessIgnoresInterruptSignal
	ProcessHasNotStartedYet
)

type SupervisorEvent struct {
	EventType   EventType
	EventReason EventReason
	Id          string
	Pool        string
	Cmd         *cmd.Status
}

// processStartupInfo contains all the static information related to starting the process.
type processStartupInfo struct {
	name          string
	args          []string
	signalTimeout time.Duration
	maxRestarts   int
}

// processStartupInfo contains all the information related to the process runtime.
type processRuntimeInfo struct {
	cmd           *cmd.Cmd
	startTime     time.Time
	stdStreamChan chan struct{}
}

type Limiter interface {
	// Start monitoring a process and pass an event to the ch when the limit is exceeded.
	Monitor(ch chan SupervisorEvent, ctx context.Context, cmd *cmd.Cmd)
	// Returns true if the limiter is currently monitoring a process.
	IsMonitoring() bool
}

type Worker interface {
	// Stream the stdout/stderr to the logger
	StreamLogs(logger *zap.Logger, outW io.Writer, errW io.Writer)
	// Run the process
	// TODO(monocerosdev): pass the logger in the constructor of the object
	// or limit it to just StreamLogs
	Run(logger *zap.Logger)
	// Wait for the worker (blocks) to empty the stdout/stderr channels
	WaitForWorkerToCleanBuffer() error
	// Get the status of the worker
	Status() ProcessStatus
	// Gracefully tries to terminate the process and pushes a signal if it has failed after timeout
	Terminate(timeout time.Duration)
	// Get the number of restarts the worker has done
	Restarts() int
	// Send a SIGKILL to the process
	ForceTerminateWorker()
}

type WorkerIntConfig struct {
	logger                *zap.Logger
	name                  string
	poolName              string
	ch                    chan SupervisorEvent
	opt                   cmd.Options
	execName              string
	args                  []string
	restarts              int
	memory                *Limiter
	lifespan              *Limiter
	collectProcessMetrics bool
}

type NewWorkerFunc = func(opt WorkerIntConfig) Worker
