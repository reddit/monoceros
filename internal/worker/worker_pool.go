package worker

import (
	"bytes"
	"fmt"
	"html/template"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"time"

	"github.com/reddit/monoceros/metrics"

	"github.com/go-cmd/cmd"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/reddit/monoceros/internal/common"
	"go.uber.org/zap"
)

func commandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

const ProxyId = "monoceros-envoy"
const envoyExec = "envoy"

type poolState = int

// Tracks the state of the pool, used to assert that operations
// are called on the correct state
const (
	Init poolState = iota
	Prepared
	Started
	Draining
	Stopped
)

type serverInfo struct {
	ports                map[string]uint64
	portArgName          string
	valueTemplate        *template.Template
	startPort            uint64
	proxyStartup         processStartupInfo
	loadBalanceAlgorithm string
	healthCheckPath      string
	envoyGen             EnvoyConfigGenerator
	proxyAdminPort       uint64
	proxyListenerPort    uint64
}

func (p *serverInfo) getPortArgs(workerName string) ([]string, error) {
	var args []string
	var buffer bytes.Buffer
	data := struct {
		Port int
	}{
		Port: int(p.ports[workerName]),
	}
	err := p.valueTemplate.Execute(&buffer, data)
	if err != nil {
		return nil, err
	}
	args = []string{p.portArgName, buffer.String()}
	return args, nil
}

type WorkerPool struct {
	// Name of the worker pool
	poolName string
	// Information used to start a worker in the pool
	startup processStartupInfo
	// Information used by the pool to control startup bursts
	startupWaitTime time.Duration
	// Each worker pool manages multiple copies of a single worker
	workers map[string]Worker
	// Number of replicas of the process
	replicas int
	// Injected logger
	logger *zap.Logger
	// Factory method used to construct workers.
	// Used by tests to set in-memory workers that have a "richer" state and more knobs
	workerFactory NewWorkerFunc
	// Current state of the pool
	state poolState
	// Limiters applied to all replicas in the pool
	maxLifespan    time.Duration
	jitterLifespan float64
	// Channel used to get events from the workers
	aggregateCh chan SupervisorEvent
	// desiredHealthPcnt describes the percent of workers needed to be up and running in order to be considered healthy
	desiredHealthPcnt int
	// Each worker is assigned to a specific port
	sInfo *serverInfo
}

func NewWorkerPool(c WorkerConfig,
	ch chan SupervisorEvent,
	logger *zap.Logger,
	workerFactory NewWorkerFunc) (*WorkerPool, error) {
	if !commandExists(c.Exec) {
		return nil, fmt.Errorf("%w: name %s", errExecutableNotFound, c.Exec)
	}

	if logger == nil {
		return nil, fmt.Errorf("%w: name logger", common.ErrRequirementParameterMissing)
	}

	if c.Replicas <= 0 {
		return nil, fmt.Errorf("%w: replicas can not be <= 0", common.ErrRequirementParameterInvalid)
	}

	sw := &WorkerPool{
		logger: logger,
		state:  Init,
		startup: processStartupInfo{
			name:          c.Exec,
			args:          c.Args,
			signalTimeout: c.SignalTimeoutOrDefault(),
			maxRestarts:   c.MaxRestartsOrDefault(),
		},
		startupWaitTime:   c.StartupWaitTime,
		workerFactory:     workerFactory,
		replicas:          c.Replicas,
		poolName:          c.Name,
		workers:           make(map[string]Worker),
		aggregateCh:       ch,
		desiredHealthPcnt: c.DesiredHealthyPercentage,
	}

	sw.workerFactory = workerFactory
	sw.replicas = c.Replicas
	sw.poolName = c.Name
	sw.workers = make(map[string]Worker)

	if c.Server != nil {
		if c.Server.PortArg.ValueTemplate == "" {
			return nil, fmt.Errorf("Port value template can not be null")
		}

		t, err := template.New("port_value").Parse(c.Server.PortArg.ValueTemplate)
		if err != nil {
			return nil, fmt.Errorf("Cannot construct worker due to ill-formatted template: %v", err)
		}

		// We drain the proxy for as long as the workers themselves are draining
		drainTime := c.SignalTimeoutOrDefault()
		sw.sInfo = &serverInfo{
			ports:         make(map[string]uint64),
			startPort:     c.Server.PortArg.StartPort,
			portArgName:   c.Server.PortArg.Name,
			valueTemplate: t,
			proxyStartup: processStartupInfo{
				name: envoyExec,
				args: []string{
					"--drain-time-s", fmt.Sprint(math.Ceil(drainTime.Seconds())),
					"--drain-strategy", "immediate",
					"-c",
					path.Join(c.Server.ProxyPathPrefixOrDefault(), envoyBootstrapConfig)},
				signalTimeout: c.Server.SignalTimeoutOrDefault(),
			},
			loadBalanceAlgorithm: c.Server.LoadBalancerAlgorithmOrDefault(),
			healthCheckPath:      c.Server.HealthCheckPath,
			envoyGen: EnvoyConfigGenerator{
				pathPrefix: c.Server.ProxyPathPrefixOrDefault(),
			},
			proxyAdminPort:    uint64(c.Server.ProxyAdminPort),
			proxyListenerPort: c.Server.ProxyListenerPortOrDefault(),
		}
	}

	if c.Limits != nil && c.Limits.Lifespan != 0 {
		sw.jitterLifespan = c.Limits.JitterRatio
		sw.maxLifespan = c.Limits.Lifespan
	}

	return sw, nil
}

// PrepareWorkers prepares the pool to be able to start the workers
func (sw *WorkerPool) PrepareWorkers(cmdOpts cmd.Options) error {
	sw.state = Prepared
	for i := 0; i < sw.replicas; i++ {
		workerName := fmt.Sprintf("%s-%d", sw.poolName, i)
		if _, ok := sw.workers[workerName]; ok {
			return fmt.Errorf("%w: collision at worker pool %s with worker %s",
				common.ErrDuplicateMapEntry, sw.poolName, workerName)
		}

		if sw.sInfo != nil {
			sw.sInfo.ports[workerName] = sw.sInfo.startPort + uint64(i)
		}

		sw.prepareWorkerInternal(cmdOpts, workerName, 0)
		// Initialize metrics at 0.
		restartsCounter.With(prometheus.Labels{
			poolLabel:  sw.poolName,
			workerType: "worker",
		})

		restartsCounter.With(prometheus.Labels{
			poolLabel:  sw.poolName,
			workerType: "proxy",
		})
		sigkillCounter.With(prometheus.Labels{
			poolLabel: sw.poolName,
		})
	}

	if sw.sInfo != nil {
		err := sw.prepareProxy(cmd.Options{
			Buffered:  false,
			Streaming: false,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func (sw *WorkerPool) RunningCount() int {
	t := 0
	for _, w := range sw.workers {
		// Don't interrupt proxy now, defer till all workers
		// are done for later.
		if w.Status() == Running {
			t += 1
		}
	}
	return t
}

func (sw *WorkerPool) InterruptAll() {
	sw.state = Draining
	if sw.HasProxy() {
		err := sw.proxyDrain()
		if err != nil {
			sw.logger.Info("Error when sending drain request", zap.Error(err))
		}
	}

	for id, w := range sw.workers {
		// Don't interrupt proxy now, defer till all workers
		// are done for later.
		if id != ProxyId {
			w.Terminate(sw.startup.signalTimeout)
		}
	}
}

func (sw *WorkerPool) RunAll() {
	sw.logger.Debug("Starting worker pool", zap.String("name", sw.poolName))

	for _, w := range sw.workers {
		<-time.After(sw.startupWaitTime)
		w.Run(sw.logger)
		metrics.AddRunningStats(sw.Name(), 1)
	}
}

func (sw *WorkerPool) RestartWorker(cmdOpts cmd.Options, id string) error {
	if sw.state == Draining || sw.state == Stopped {
		return nil
	}

	restarts := sw.workers[id].Restarts() + 1

	if id == ProxyId {
		err := sw.prepareProxy(cmdOpts)
		if err != nil {
			return err
		}
	} else {
		if restarts > sw.startup.maxRestarts {
			return fmt.Errorf("%w: too many restarts in worker pool %s", ErrTooManyRestarts, sw.poolName)
		}

		sw.prepareWorkerInternal(cmdOpts, id, restarts)
	}

	wType := "worker"
	if id == ProxyId {
		wType = "proxy"
	}

	restartsCounter.With(prometheus.Labels{
		poolLabel: sw.poolName,
		// We annotate the worker type because if proxy restarts it is a monoceros issues
		// and if the worker restarts it is an application issue.
		workerType: wType,
	}).Inc()

	sw.workers[id].Run(sw.logger)
	return nil
}

func (sw *WorkerPool) prepareWorkerInternal(cmdOpts cmd.Options, workerName string, restarts int) {
	var args []string
	if sw.sInfo != nil {
		portArgs, err := sw.sInfo.getPortArgs(workerName)
		if err != nil {
			panic(err)
		}
		args = portArgs
	}

	args = append(args, sw.startup.args...)
	wconf := WorkerIntConfig{
		logger:                sw.logger,
		name:                  workerName,
		poolName:              sw.poolName,
		ch:                    sw.aggregateCh,
		opt:                   cmdOpts,
		execName:              sw.startup.name,
		args:                  args,
		restarts:              restarts,
		collectProcessMetrics: true,
	}

	if sw.maxLifespan != 0 {
		t := newTimeLimiter(workerName, sw.poolName, sw.maxLifespan, sw.jitterLifespan)
		wconf.lifespan = &t
	}

	sw.logger.Debug("Preparing worker pool", zap.String("exec", sw.startup.name), zap.Strings("args", args))
	w := sw.workerFactory(wconf)
	go w.StreamLogs(sw.logger, os.Stdout, os.Stderr)
	sw.workers[workerName] = w
}

func (sw *WorkerPool) WaitForWorkerToCleanBuffer(id string) error {
	return sw.workers[id].WaitForWorkerToCleanBuffer()
}

func (sw *WorkerPool) InterruptWorker(id string) {
	sw.workers[id].Terminate(sw.startup.signalTimeout)
}

func (sw *WorkerPool) ForceTerminateWorker(id string) {
	sigkillCounter.With(prometheus.Labels{
		poolLabel: sw.poolName,
	}).Inc()
	sw.workers[id].ForceTerminateWorker()
}

func (sw *WorkerPool) Terminated() bool {
	// check if all process have stopped
	for _, w := range sw.workers {
		if w.Status() != Terminated {
			return false
		}
	}
	return true
}

func (sw *WorkerPool) Len() int {
	return sw.replicas
}

func (sw *WorkerPool) Name() string {
	return sw.poolName
}

func (sw *WorkerPool) IsHealthy() bool {
	var running int
	for _, w := range sw.workers {
		if w.Status() == Running {
			running++
		}
	}
	healthPercentage := int((float32(running) / float32(len(sw.workers))) * 100)
	return healthPercentage >= sw.desiredHealthPcnt
}

func (sw *WorkerPool) prepareProxy(cmdOpts cmd.Options) error {
	wconf := WorkerIntConfig{
		logger:   sw.logger,
		name:     ProxyId,
		poolName: sw.poolName,
		ch:       sw.aggregateCh,
		opt:      cmdOpts,
		execName: sw.sInfo.proxyStartup.name,
		args:     sw.sInfo.proxyStartup.args,
		// We don't want metrics for the proxy
		collectProcessMetrics: false,
	}

	// Write Cluster Config to disk
	portRange := make([]uint64, sw.replicas)
	for i := range portRange {
		portRange[i] = sw.sInfo.startPort + uint64(i)
	}

	opt := EnvoyClusterOpt{
		LoadBalanceAlgorithm: sw.sInfo.loadBalanceAlgorithm,
		PortRange:            portRange,
		HealthCheckPath:      sw.sInfo.healthCheckPath,
	}
	err := sw.sInfo.envoyGen.RenderClustersToWriter(opt)
	if err != nil {
		return err
	}

	// Write Listener Config to disk
	lOpt := EnvoyListenerOpt{
		Port: sw.sInfo.proxyListenerPort,
	}
	err = sw.sInfo.envoyGen.RenderListenersToWriter(lOpt)
	if err != nil {
		return err
	}

	// Write Bootstrap Config to disk
	bOpt := EnvoyBootstrapOpt{
		Port: sw.sInfo.proxyAdminPort,
	}
	err = sw.sInfo.envoyGen.RenderBootstrapConfig(bOpt)
	if err != nil {
		return err
	}

	sw.logger.Debug("Preparing worker pool proxy", zap.String("Proxy", ProxyId))
	sw.workers[ProxyId] = sw.workerFactory(wconf)
	go sw.workers[ProxyId].StreamLogs(sw.logger, os.Stdout, os.Stderr)
	return nil
}

func (sw *WorkerPool) proxyDrain() error {
	addr := "http://" + net.JoinHostPort("127.0.0.1", fmt.Sprint(sw.sInfo.proxyAdminPort)) + "/drain_listeners?graceful"
	client := &http.Client{}
	req, err := http.NewRequest("POST", addr, nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()
	return nil
}

func (sw *WorkerPool) HasProxy() bool {
	return sw.sInfo != nil
}
