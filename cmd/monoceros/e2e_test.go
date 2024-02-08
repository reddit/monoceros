package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/procfs"
	"github.com/reddit/monoceros/internal/admin"
	"github.com/reddit/monoceros/internal/config"
	"github.com/reddit/monoceros/internal/supervisor"
	"github.com/reddit/monoceros/internal/worker"
	"go.uber.org/zap/zaptest"
)

func canCollectProcess() bool {
	_, err := procfs.NewDefaultFS()
	return err == nil
}

func TestProcessCollectorStats(t *testing.T) {
	t.Run("Should have e2e process collection stats", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping integration test")
		}

		if !canCollectProcess() {
			t.Skip("Test is not supported in this platform")
		}

		ch := make(chan bool)
		go func() {
			err := mainCore(&config.MonocerosConfig{
				AdminConfig: admin.AdminConfig{
					Port: 9000,
				},
				SupervisorConfig: supervisor.SupervisorConfig{
					Workers: []worker.WorkerConfig{
						{
							Name:          "processbin",
							Exec:          "../../dist/processbin",
							Args:          []string{"--exit-after", "5"},
							MaxRestarts:   0,
							Replicas:      1,
							SignalTimeout: 15 * time.Second,
						},
					},
				},
			}, zaptest.NewLogger(t))
			if err != nil {
				t.Errorf("%s: unexpected error %v running mainCore", t.Name(), err)
			}
			ch <- true
		}()

		serverStarted := false
		for start := time.Now(); time.Since(start) < 15*time.Second; {
			resp, err := http.Get("http://0.0.0.0:9000/metrics")
			if err == nil && resp.StatusCode == 200 {
				defer resp.Body.Close()
				serverStarted = true
				bodyBytes, err := io.ReadAll(resp.Body)
				if err != nil {
					fmt.Println(err)
				}

				if !(strings.Contains(string(bodyBytes), "process_cpu_seconds_total")) {
					t.Errorf("%s: admin endpoint did not contain process_cpu_seconds_total", t.Name())
				}

				if !(strings.Contains(string(bodyBytes), "process_open_fds")) {
					t.Errorf("%s: admin endpoint did not contain process_open_fds", t.Name())
				}

				break
			}
		}

		if !serverStarted {
			t.Fatalf("%s: admin endpoint could not serve metrics", t.Name())
		}
		<-ch
	})
}
