package worker

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	promNamespace = "monoceros"
	subsystem     = "workerpool"

	poolLabel   = "pool"
	workerLabel = "worker"
	typeLabel   = "limit_event"
	stateLabel  = "state"
	workerType  = "worker_type"
)

var (
	restartsLabels = []string{
		poolLabel,
		workerType,
	}

	// Counters.
	restartsCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: subsystem,
		Name:      "restarts_total",
		Help:      "Number of times a worker process has been restarted",
	}, restartsLabels)
)

var (
	sigkillLabel = []string{
		poolLabel,
	}

	// Counters.
	sigkillCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: subsystem,
		Name:      "sigkill_total",
		Help:      "Number of times a worker did not answer to SIGTERM on time and we send it a SIGKILL",
	}, sigkillLabel)
)

var (
	limitLabels = []string{
		poolLabel, typeLabel,
	}

	limitCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: subsystem,
		Name:      "limits_events_total",
		Help:      "Number of times limits have been enforced on a worker",
	}, limitLabels)
)

var (
	failedToParseLogsCounter = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: promNamespace,
		Subsystem: subsystem,
		Name:      "failed_to_parse_log_json_total",
		Help:      "Number of times we have failed to parse the log line as JSON, even tho it is valid JSON",
	})
)
