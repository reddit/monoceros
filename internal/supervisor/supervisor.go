package supervisor

import (
	"context"
	"errors"
	"fmt"

	"github.com/reddit/monoceros/metrics"

	"github.com/go-cmd/cmd"
	"github.com/reddit/monoceros/internal/common"
	"github.com/reddit/monoceros/internal/worker"
	"go.uber.org/zap"
)

type WorkerPoolInfo struct {
	pool WorkerPoolAbstraction
	// These counters are used to perform some accounting that is useful
	// during shutdown. We want to shutdown the proxy after all the workers
	// are done. So we keep track of the `runningProcesses` and when this drops
	// and is equal to 1 we terminate the proxy (last running process).
	runningProcesses int
	totalProcesses   int
}

type Supervisor struct {
	terminating bool
	logger      *zap.Logger
	// Used to inject different worker types in tests
	newWorkerFunc worker.NewWorkerFunc
	pools         map[string]WorkerPoolInfo
	config        SupervisorConfig
	cmdOptions    cmd.Options
	ch            chan worker.SupervisorEvent
}

type WorkerPoolAbstraction interface {
	RunAll()
	InterruptWorker(id string)
	InterruptAll()
	ForceTerminateWorker(id string)
	WaitForWorkerToCleanBuffer(id string) error
	RestartWorker(options cmd.Options, id string) error
	IsHealthy() bool
	HasProxy() bool
	RunningCount() int
}

func NewSupervisor(config SupervisorConfig,
	newWorkerFunc worker.NewWorkerFunc,
	logger *zap.Logger) (*Supervisor, error) {
	if logger == nil {
		return nil, fmt.Errorf("%w: name logger", common.ErrRequirementParameterMissing)
	}

	cmdOptions := cmd.Options{
		Buffered:  false,
		Streaming: true,
	}
	ch := make(chan worker.SupervisorEvent)

	return &Supervisor{
		terminating:   false,
		logger:        logger,
		newWorkerFunc: newWorkerFunc,
		config:        config,
		cmdOptions:    cmdOptions,
		ch:            ch,
	}, nil
}

func (s *Supervisor) Run(ctx context.Context, cancel context.CancelFunc) error {
	terminatedProcesses := 0
	pInfo, totalProcesses, err := s.createAndPreparePools()
	if err != nil {
		return err
	}
	s.pools = pInfo

	// Start all workers in all pools
	for _, p := range s.pools {
		p.pool.RunAll()
	}

	for !s.terminating {
		select {
		case event := <-s.ch:
			switch event.EventType {
			case worker.ProcessEnd:
				err := s.handleProcessEnd(event)
				if err != nil {
					if errors.Is(err, worker.ErrTooManyRestarts) {
						cancel()
					}
					terminatedProcesses += 1
					s.handleProcessTerminationDuringShutdown(event)
				} else {
					metrics.IncrementRunningDecrementTerminated(event.Pool)
					s.logger.Info("Restart",
						zap.String("pool", event.Pool),
						zap.String("worker_id", event.Id),
						zap.Int("rc", event.Cmd.Exit))
				}
			case worker.ProcessExceededLimit:
				s.handleProcessExceededLimit(event)
			case worker.ProcessNeedsInterrupt:
				s.handleProcessNeedsInterrupt(event)
			case worker.ProcessNeedsTermination:
				s.handleProcessNeedsTermination(event)
			}
		case <-ctx.Done():
			metrics.SetSigtermGauge()
			s.logger.Info("Interrupt caught, cleaning up workers. This might take some time...")
			s.terminating = true
		}
	}

	// Interrupt all workers here
	for _, p := range s.pools {
		p.pool.InterruptAll()
	}

	// We check here if we are done and we don't need to wait on more workers
	if terminatedProcesses == totalProcesses {
		return nil
	}

	// Read from events until all process are done
	// TODO(monocerosdev): we should be respectful of the workers but the supervisor should not hang in case of a bug
	// so we should make sure at some point that we eventually return even if the pools have not drained
	for terminatedProcesses != totalProcesses {
		event := <-s.ch
		switch event.EventType {
		case worker.ProcessEnd:
			err := s.handleProcessEnd(event)
			if err != nil {
				terminatedProcesses += 1
				s.handleProcessTerminationDuringShutdown(event)
			}
		case worker.ProcessExceededLimit:
		case worker.ProcessNeedsInterrupt:
			s.handleProcessNeedsInterrupt(event)
		case worker.ProcessNeedsTermination:
			s.handleProcessNeedsTermination(event)
		}
	}

	// Sanity check that termination worked correctly.
	// At this point we might as well panic and error out here if
	// cleaned up was leaky
	sum := 0
	for _, p := range s.pools {
		sum += p.pool.RunningCount()
	}

	if sum != 0 {
		return fmt.Errorf("monoceros cleanup went wrong: %d-%d", sum, 0)
	}

	return nil
}

func (s *Supervisor) createAndPreparePools() (map[string]WorkerPoolInfo, int, error) {
	totalProcesses := 0
	pools := make(map[string]WorkerPoolInfo)
	for _, w := range s.config.Workers {
		totalProcesses += w.Replicas
		if _, ok := pools[w.Name]; ok {
			err := fmt.Errorf("%w: collision detected workers with the same name %s", errDuplicateWorkerName, w.Name)
			return nil, totalProcesses, err
		}
		pool, err := worker.NewWorkerPool(w, s.ch, s.logger, s.newWorkerFunc)
		if err != nil {
			// We generate these erors ourselves so we dont need to decorate them
			return nil, totalProcesses, err
		}
		err = pool.PrepareWorkers(s.cmdOptions)
		if err != nil {
			// We generate these erors ourselves so we dont need to decorate them
			return nil, totalProcesses, err
		}

		proxyProcess := 0
		if pool.HasProxy() {
			proxyProcess = 1
			metrics.SetProxyModeGauge()
		}
		totalProcesses += proxyProcess
		totalProcessesPool := w.Replicas + proxyProcess
		pools[w.Name] = WorkerPoolInfo{
			pool:             pool,
			totalProcesses:   totalProcessesPool,
			runningProcesses: totalProcessesPool,
		}
	}
	return pools, totalProcesses, nil
}

func (s *Supervisor) handleProcessTerminationDuringShutdown(event worker.SupervisorEvent) {
	pInfo := s.pools[event.Pool]
	pInfo.runningProcesses -= 1
	if pInfo.pool.HasProxy() && pInfo.runningProcesses == (pInfo.totalProcesses-1) {
		s.logger.Info("Terminating proxy during shutdown",
			zap.String(common.WorkerPoolLogLabel, event.Pool))
		pInfo.pool.InterruptWorker(worker.ProxyId)
	}
	s.pools[event.Pool] = pInfo
}

func (s *Supervisor) handleProcessExceededLimit(e worker.SupervisorEvent) {
	s.logger.Info("Stopping worker because it exceeds limit",
		zap.Int("LimitType", int(e.EventReason)),
		zap.String(common.WorkerPoolLogLabel, e.Pool),
		zap.String(common.WorkerIdLogLabel, e.Id))
	s.pools[e.Pool].pool.InterruptWorker(e.Id)
}

func (s *Supervisor) handleProcessNeedsInterrupt(e worker.SupervisorEvent) {
	s.logger.Info("Interrupting again",
		zap.Int("reason", int(e.EventReason)),
		zap.String(common.WorkerPoolLogLabel, e.Pool),
		zap.String(common.WorkerIdLogLabel, e.Id))
	s.pools[e.Pool].pool.InterruptWorker(e.Id)
}

func (s *Supervisor) handleProcessNeedsTermination(e worker.SupervisorEvent) {
	s.logger.Info("Terminating worker forcefully",
		zap.Int("reason", int(e.EventReason)),
		zap.String(common.WorkerPoolLogLabel, e.Pool),
		zap.String(common.WorkerIdLogLabel, e.Id))
	s.pools[e.Pool].pool.ForceTerminateWorker(e.Id)
}

func (s *Supervisor) handleProcessEnd(e worker.SupervisorEvent) error {
	// Restart the process
	// wait for the process to finish with the logging
	// TODO(monocerosdev): This blocks the entire for select statement here which can be bad potentially.
	// But I am not sure if this matters in practice.
	// Since `e.eventType == ProcessEnd` we have already received the event that the process has terminated
	// So while we block there is no-one on the other side than can makes wait indefinitely
	// we just need to empty the stdout/stderr buffers

	err := s.pools[e.Pool].pool.WaitForWorkerToCleanBuffer(e.Id)
	if err != nil {
		// The internal state is invalid here, we should panic.
		panic(err)
	}

	metrics.DecrementRunningIncrementTerminated(e.Pool)
	if s.terminating {
		// We don't try to restart processes if the supervisor is shutting down.
		return errors.New("Supervisor is already terminating")
	}

	err = s.pools[e.Pool].pool.RestartWorker(s.cmdOptions, e.Id)
	if errors.Is(err, worker.ErrTooManyRestarts) {
		s.logger.Info("Too many restarts", zap.String("pool", e.Pool), zap.String("worker_id", e.Id))
	}

	return err
}

func (s *Supervisor) IsHealthy() bool {
	for _, p := range s.pools {
		if !p.pool.IsHealthy() {
			return false
		}
	}
	return true
}
