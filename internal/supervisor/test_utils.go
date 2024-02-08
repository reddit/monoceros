package supervisor

import (
	"io"
	"strings"
	"testing"

	"github.com/go-cmd/cmd"
)

type mockWorkerPool struct {
	runAll                     func()
	interruptWorker            func(id string)
	interruptAll               func()
	forceTerminateWorker       func(id string)
	waitForWorkerToCleanBuffer func(id string) error
	restartWorker              func(options cmd.Options, id string) error
	isHealthy                  func() bool
	hasProxy                   func() bool
	runningCount               func() int
}

func (m mockWorkerPool) RunAll() {
	m.runAll()
}

func (m mockWorkerPool) InterruptWorker(id string) {
	m.interruptWorker(id)
}

func (m mockWorkerPool) InterruptAll() {
	m.interruptAll()
}

func (m mockWorkerPool) ForceTerminateWorker(id string) {
	m.forceTerminateWorker(id)
}

func (m mockWorkerPool) WaitForWorkerToCleanBuffer(id string) error {
	return m.waitForWorkerToCleanBuffer(id)
}

func (m mockWorkerPool) RestartWorker(options cmd.Options, id string) error {
	return m.restartWorker(options, id)
}

func (m mockWorkerPool) IsHealthy() bool {
	return m.isHealthy()
}

func (m mockWorkerPool) HasProxy() bool {
	return m.hasProxy()
}

func (m mockWorkerPool) RunningCount() int {
	return m.runningCount()
}

func validateProcessbinResponse(body io.ReadCloser, t *testing.T) {
	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		t.Errorf("%s: validateProcessbinResponse %v", t.Name(), err)
	}

	wantString := "hello from processbin"
	if !(strings.Contains(string(bodyBytes), wantString)) {
		t.Errorf("%s: worker response did not contain '%s'", t.Name(), wantString)
	}
}
