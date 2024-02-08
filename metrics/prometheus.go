package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	promNamespace       = "monoceros"
	workerPoolSubSystem = "workerpool"
	supervisorSubSystem = "supervisor"

	poolLabel  = "pool"
	stateLabel = "state"
)

// Worker state labels.
const (
	statePrepared   = "prepared"
	stateRunning    = "running"
	stateTerminated = "terminated"
)

// This gauge is primarily managed by the supervisor since the supervisor subscribes
// to all the events coming from the workers, except for initial startup.
// The worker_pool contains the information needed to start a process (startup info), and
// is not aware of the lifetime of the process. This makes it hard to implement it there
// The worker has all the runtime information of the process but is not aware of the restarts
// since each restart creating a new worker. This makes it tricky to decrement the terminated counter.
var (
	workerStateLabels = []string{
		poolLabel, stateLabel,
	}

	workersStateGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: promNamespace,
			Subsystem: workerPoolSubSystem,
			Name:      "worker_state",
			Help:      "Number of workers in a particular state",
		}, workerStateLabels)

	supervisorSigtermGauge = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: promNamespace,
			Subsystem: supervisorSubSystem,
			Name:      "sigterm_received",
			Help:      "Supervisor current state, it becomes 1 when the supervisor receives a SIGTERM",
		})

	supervisorProxyMode = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: promNamespace,
			Subsystem: supervisorSubSystem,
			Name:      "proxy_mode",
			Help:      "Supervisor proxy mode, it becomes 1 if monoceros is running on proxy mode",
		})
)

// AddRunningStats increments the running stats by the number.
func AddRunningStats(pool string, number int) {
	workersStateGauge.With(statsLabels(pool, stateRunning)).Add(float64(number))
}

// IncrementRunningDecrementTerminated increments a running gauge and
// decrements the terminated gauge.
func IncrementRunningDecrementTerminated(pool string) {
	workersStateGauge.With(statsLabels(pool, stateTerminated)).Dec()
	workersStateGauge.With(statsLabels(pool, stateRunning)).Inc()
}

// DecrementRunningIncrementTerminated decrements a running gauge and
// increments the terminated gauge.
func DecrementRunningIncrementTerminated(pool string) {
	workersStateGauge.With(statsLabels(pool, stateRunning)).Dec()
	workersStateGauge.With(statsLabels(pool, stateTerminated)).Inc()
}

func SetSigtermGauge() {
	supervisorSigtermGauge.Set(1)
}

func SetProxyModeGauge() {
	supervisorProxyMode.Set(1)
}

func statsLabels(pool, state string) prometheus.Labels {
	return prometheus.Labels{
		poolLabel:  pool,
		stateLabel: state,
	}
}
