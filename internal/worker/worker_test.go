package worker

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-cmd/cmd"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
	"go.uber.org/zap/zaptest/observer"
)

var processBinPath = "../../dist/processbin"

func validateProcessRunsToCompletion(t *testing.T, w *realProcessWorker) {
	status := <-w.channel
	err := w.WaitForWorkerToCleanBuffer()
	if err != nil {
		t.Errorf("validateProcessCompletes test %s unexpected error %v while waiting for stdstream buff",
			t.Name(), err)
	}

	if got, want := status.EventType, ProcessEnd; got != want {
		t.Errorf("validateProcessCompletes test %s =  got event %d, want %d", t.Name(), got, want)
	}

	if got, want := status.Cmd.Exit, 0; got != want {
		t.Errorf("validateProcessCompletes test %s  got exit code %d, want %d", t.Name(), got, 0)
	}
}

type logItem struct {
	logMessage string
	level      zapcore.Level
	seen       bool
}

func validateLogExists(t *testing.T, logs *observer.ObservedLogs, items map[string]logItem) {
	for _, log := range logs.All() {
		if len(log.Context) == 0 {
			var objmap map[string]json.RawMessage
			_ = json.Unmarshal(json.RawMessage(log.Message), &objmap)
			for key, val := range objmap {
				log.Context = append(log.Context, zapcore.Field{
					Key:       key,
					Interface: val,
				})
			}
		}

		for _, entry := range log.Context {
			if val, ok := items[entry.Key]; ok {
				// In some cases we log string and in some cases we log the json as is
				// so we optimistically try to get the entry string and if this is not
				// correct we get the interface and assume it is a json object.
				got := entry.String
				want := val.logMessage
				if len(got) == 0 && len(want) != 0 {
					got = string(entry.Interface.(json.RawMessage))
				}

				if got != want {
					t.Errorf("validateLogExists test %s got %s, want %s", t.Name(), got, want)
				}
				if got, want := log.Level, val.level; got != want {
					t.Errorf("validateLogExists test %s got level %d, want %d", t.Name(), got, want)
				}
				val.seen = true
				items[entry.Key] = val
			}
		}
	}

	for k, v := range items {
		if !v.seen {
			t.Errorf("validateLogExists test %s doesnt contain field %s", t.Name(), k)
		}
	}
}

func getTestWorkerInstance(t *testing.T, args []string, logger *zap.Logger) realProcessWorker {
	streamStdOut := true
	if logger == nil {
		streamStdOut = false
		logger = zaptest.NewLogger(t)
	}
	wconf := WorkerIntConfig{
		logger:   logger,
		name:     "worker",
		poolName: "pool",
		ch:       make(chan SupervisorEvent),
		opt: cmd.Options{
			Buffered:  false,
			Streaming: streamStdOut,
		},
		execName: processBinPath,
		args:     args,
		restarts: 0,
	}
	return newWorkerInstance(wconf)
}

// TODO: Issue #69 part 2
//
//nolint:gocyclo
func TestWorkerBasicIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	t.Run("Should start and end processes", func(t *testing.T) {
		args := []string{"--exit-after", "1"}
		worker := getTestWorkerInstance(t, args, nil)
		go worker.Run(zaptest.NewLogger(t))
		validateProcessRunsToCompletion(t, &worker)
	})

	t.Run("Should capture stdout and log it", func(t *testing.T) {
		observed, logs := observer.New(zapcore.InfoLevel)
		logger := zap.New(zapcore.NewTee(observed))

		logMessage := "a log"
		args := []string{"--exit-after", "1", "--log-fixed", logMessage, "--log-interval", "100"}
		worker := getTestWorkerInstance(t, args, logger)
		go worker.StreamLogs(logger, os.Stdout, os.Stderr)
		go worker.Run(logger)
		validateProcessRunsToCompletion(t, &worker)
		validateLogExists(t, logs, map[string]logItem{
			"worker_info_msg": {
				logMessage: logMessage,
				level:      zapcore.InfoLevel,
			},
		})
	})

	t.Run("Should capture stderr and log it as stderr", func(t *testing.T) {
		observed, logs := observer.New(zapcore.InfoLevel)
		logger := zap.New(zapcore.NewTee(observed))

		logMessage := "a log"
		args := []string{"--exit-after", "1", "--log-error", "--log-fixed", logMessage, "--log-interval", "100"}
		worker := getTestWorkerInstance(t, args, logger)
		go worker.StreamLogs(logger, os.Stdout, os.Stderr)
		go worker.Run(logger)
		validateProcessRunsToCompletion(t, &worker)
		validateLogExists(t, logs, map[string]logItem{
			"worker_info_msg": {
				logMessage: logMessage,
				level:      zapcore.InfoLevel,
			},
			"worker_error_msg": {
				logMessage: logMessage,
				level:      zapcore.ErrorLevel,
			},
		})
	})

	type ExpectedStdoutJsonStruct struct {
		A int    `json:"a"`
		W string `json:"worker_id"`
	}

	t.Run("Should capture json stdout and log it", func(t *testing.T) {
		observed, _ := observer.New(zapcore.InfoLevel)
		logger := zap.New(zapcore.NewTee(observed))

		logMessage := `{"a" : 1}`
		args := []string{"--exit-after", "1", "--log-fixed", logMessage, "--log-interval", "100"}
		worker := getTestWorkerInstance(t, args, logger)
		var b bytes.Buffer
		outW := bufio.NewWriter(&b)
		go worker.StreamLogs(logger, outW, outW)
		go worker.Run(logger)
		validateProcessRunsToCompletion(t, &worker)
		outW.Flush()
		out := ExpectedStdoutJsonStruct{}
		err := json.Unmarshal(b.Bytes(), &out)
		if err != nil {
			t.Fatalf("%s unexpected error when unmarshalling stdout %v", t.Name(), err)
		}

		if got, want := out.A, 1; got != want {
			t.Errorf("test %s json stdout validation: got %d, want %d, raw json %s", t.Name(), got, want, b.Bytes())
		}

		if got, want := out.W, worker.name; got != want {
			t.Errorf("test %s json stdout validation:  got %s, want %s, raw json: %s", t.Name(), got, want, b.Bytes())
		}

	})

	// TODO(monocerosdev): this not a feature just a test to document a known behavior
	// determine if this is a blocker or not.
	t.Run("Should drop when stdout log line is > 16384", func(t *testing.T) {
		observed, logs := observer.New(zapcore.InfoLevel)
		logger := zap.New(zapcore.NewTee(observed))
		args := []string{"--exit-after", "1", "--log-size", "16387", "--log-interval", "100"}
		worker := getTestWorkerInstance(t, args, logger)
		go worker.StreamLogs(logger, os.Stdout, os.Stderr)
		go worker.Run(logger)
		validateProcessRunsToCompletion(t, &worker)
		if got, want := len(logs.All()), 1; got != want {
			for _, info := range logs.All() {
				// This is useful for debugging the test if it fails
				fmt.Println(info)
			}
			t.Errorf("%s got %d log lines, want %d", t.Name(), got, want)
		}
	})

	t.Run("Should stop processes that accept SIGTERM", func(t *testing.T) {
		args := []string{"--exit-after", "30"}
		worker := getTestWorkerInstance(t, args, nil)

		go worker.Run(zaptest.NewLogger(t))

		for ok := true; ok; ok = worker.Status() != Running {
		}
		<-time.After(100 * time.Millisecond)

		worker.Terminate(3 * time.Second)
		status := <-worker.channel
		if got, want := status.EventType, ProcessEnd; got != want {
			t.Errorf("validateProcessCompletes test %s =  got event %d, want %d", t.Name(), got, want)
		}

		if got, want := status.Cmd.Exit, 143; got != want {
			t.Errorf("validateProcessCompletes test %s  got exit code %d, want %d", t.Name(), got, want)
		}
	})

	t.Run("Should set MULTIPROCESS_WORKER_ID environment flag", func(t *testing.T) {
		// We set this flag to make sure that existing variables are passed as well
		os.Setenv("TEST_FLAG", "1")
		args := []string{"--exit-after", "30"}
		worker := getTestWorkerInstance(t, args, nil)
		containsTestEnv := false
		containsMonocerosEnv := false
		for _, envKeyValueStr := range worker.runtime.cmd.Env {
			if envKeyValueStr == "" {
				continue
			}

			envKeyValue := strings.Split(envKeyValueStr, "=")
			if envKeyValue[0] == "TEST_FLAG" {
				containsTestEnv = true
			}

			if envKeyValue[0] == "MULTIPROCESS_WORKER_ID" {
				containsMonocerosEnv = true

				if !strings.Contains(envKeyValue[1], worker.name) {
					t.Errorf("test %s =  MULTIPROCESS_WORKER_ID value got %s, want %s", t.Name(), envKeyValue[1], worker.name)
				}
			}
		}

		if !containsTestEnv {
			t.Errorf("%s: process env variables don't contain TEST_FLAG", t.Name())
		}

		if !containsMonocerosEnv {
			t.Errorf("%s: process env variables don't contain MULTIPROCESS_WORKER_ID", t.Name())
		}
	})

	t.Run("Should return failures if stop process cant work", func(t *testing.T) {
		args := []string{"--exit-after", "30"}
		worker := getTestWorkerInstance(t, args, nil)

		go worker.Run(zaptest.NewLogger(t))

		worker.Terminate(3 * time.Second)
		status := <-worker.channel
		if got, want := status.EventType, ProcessNeedsInterrupt; got != want {
			t.Errorf("validateProcessCompletes test %s =  got event %d, want %d", t.Name(), got, want)
		}

		for ok := true; ok; ok = worker.Status() != Running {
		}

		<-time.After(100 * time.Millisecond)
		worker.Terminate(3 * time.Second)
		status = <-worker.channel
		if got, want := status.EventType, ProcessEnd; got != want {
			t.Errorf("validateProcessCompletes test %s =  got event %d, want %d", t.Name(), got, want)
		}
		if got, want := status.Cmd.Exit, 143; got != want {
			t.Errorf("validateProcessCompletes test %s  got exit code %d, want %d", t.Name(), got, want)
		}
	})

	t.Run("Should kill processes that accept SIGTERM", func(t *testing.T) {
		args := []string{"--exit-after", "30", "--ignore-signals"}
		worker := getTestWorkerInstance(t, args, nil)
		go worker.Run(zaptest.NewLogger(t))
		for ok := true; ok; ok = worker.Status() != Running {
		}
		<-time.After(100 * time.Millisecond)
		worker.Terminate(3 * time.Second)
		status := <-worker.channel
		if got, want := status.EventType, ProcessNeedsTermination; got != want {
			t.Errorf("validateProcessCompletes test %s =  got event %d, want %d", t.Name(), got, want)
		}

		worker.ForceTerminateWorker()
		status = <-worker.channel
		if got, want := status.EventType, ProcessEnd; got != want {
			t.Errorf("validate EventType test %s =  got event %d, want %d", t.Name(), got, want)
		}
	})

	t.Run("Should Monitor With Limiters", func(t *testing.T) {
		args := []string{"--exit-after", "1"}
		lt := newTimeLimiter("worker", "pool", 100*time.Millisecond, 0)
		worker := getTestWorkerInstance(t, args, nil)
		worker.lifespan = &lt
		if got, want := lt.IsMonitoring(), false; got != want {
			t.Errorf("IsMonitoring before run test %s  got %t, want %t", t.Name(), got, want)
		}

		go worker.Run(zaptest.NewLogger(t))

		for start := time.Now(); time.Since(start) < 100*time.Millisecond && !lt.IsMonitoring(); {
		}

		if got, want := lt.IsMonitoring(), true; got != want {
			t.Errorf("IsMonitoring after run test %s  got %t, want %t", t.Name(), got, want)
		}

		status := <-worker.channel
		if got, want := status.EventType, ProcessExceededLimit; got != want {
			t.Errorf("validate EventType test %s =  got event %d, want %d", t.Name(), got, want)
		}
		if got, want := status.EventReason, TimeLimitExceeded; got != want {
			t.Errorf("validate EventReason test %s =  got event %d, want %d", t.Name(), got, want)
		}
		worker.ForceTerminateWorker()
		status = <-worker.channel
		if got, want := status.EventType, ProcessEnd; got != want {
			t.Errorf("validate EventType test %s =  got event %d, want %d", t.Name(), got, want)
		}
	})
}
