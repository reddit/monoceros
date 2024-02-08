package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// ValidateSupervisorEndsStats is used to validate the state of the Stats when supervisor run has ended. At that time
// we should have 0 processes running and all processes marked as terminated.
func ValidateSupervisorEndsStats(t *testing.T, poolName string, replicas int) {
	if got, want := testutil.CollectAndCount(workersStateGauge), 2; got != want {
		t.Errorf("Supervisor tracks %v labels for gauge `workersStateGauge`, want %v", got, 2)
	}

	terminatedLabel := statsLabels(poolName, stateTerminated)
	if got, want := testutil.ToFloat64(workersStateGauge.With(terminatedLabel)), float64(replicas); got != want {
		t.Errorf("Supervisor tracks %v labels for gauge `workersStateGauge`, want %v", got, float64(replicas))
	}

	runningLabel := statsLabels(poolName, stateRunning)
	if got, want := testutil.ToFloat64(workersStateGauge.With(runningLabel)), 0.; got != want {
		t.Errorf("Supervisor tracks %v labels for gauge `workersStateGauge`, want %v", got, 0)
	}
}

// ValidateSupervisorStartsStats is used to validate the state of the Stats
// after supervisor run has started. At that time
// we should have all processes marked as running.
func ValidateSupervisorStartsStats(t *testing.T, poolName string, replicas int) {
	if got, want := testutil.CollectAndCount(workersStateGauge), 1; got != want {
		t.Errorf("Supervisor tracks %v labels for gauge `workersStateGauge`, want %v", got, 1)
	}

	// Although it would be better to retrieve the pool name from the Name() method,
	// this creates a data race condition during our tests.
	// Instead, for now, we can just use c.Workers[0].Name because that it is what is ultimately returned.
	runningLabel := statsLabels(poolName, stateRunning)
	if got, want := testutil.ToFloat64(workersStateGauge.With(runningLabel)), float64(replicas); got != want {
		t.Errorf("Supervisor tracks %v labels for gauge `workersStateGauge`, want %v", got, float64(replicas))
	}
}

func ValidateSupervisorSigtermStats(t *testing.T) {
	if got, want := testutil.CollectAndCount(supervisorSigtermGauge), 1; got != want {
		t.Errorf("supervisorSigtermGauge`, got %v want %v", got, 1)
	}
}

func ResetWorkerGauge() {
	workersStateGauge.Reset()
}
