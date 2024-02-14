package worker

import (
	"context"
	"math"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/go-cmd/cmd"
	"github.com/prometheus/client_golang/prometheus"
)

type TimeLimiter struct {
	timeout      time.Duration
	workerName   string
	workerPool   string
	isMonitoring atomic.Bool
}

func newTimeLimiter(wName, wPool string, t time.Duration, j float64) Limiter {
	timeout := t
	if j > 0 {
		// So we first select r (unit: ms) and the value
		// is timeout * jitter. So for example if jitter = 0.1 and
		// timeout is 100 ms then it would be 10ms
		// r is the upper bound of jitter in ms so we select a random
		// number from [0,r) (with intn) and add it to the timeout
		r := int(math.Floor(j * float64(t.Milliseconds())))
		// we do not need secure randomness here
		//nolint:gosec
		jitter := time.Duration(rand.Intn(r) * int(time.Millisecond))
		timeout += jitter
	}

	limitCounter.With(prometheus.Labels{
		poolLabel: wPool,
		typeLabel: "time",
	})

	return &TimeLimiter{
		timeout:    timeout,
		workerName: wName,
		workerPool: wPool,
	}
}

func (t *TimeLimiter) Monitor(ch chan SupervisorEvent, ctx context.Context, _ *cmd.Cmd) {
	if t.timeout == 0 {
		return
	}

	if !t.isMonitoring.CompareAndSwap(false, true) {
		return
	}

	go func(tt time.Duration, ch chan SupervisorEvent, name, pool string) {
		timer := time.NewTimer(tt)
		select {
		case <-timer.C:
			t.isMonitoring.Store(false)
			ch <- SupervisorEvent{
				EventType:   ProcessExceededLimit,
				EventReason: TimeLimitExceeded,
				Id:          name,
				Pool:        pool,
			}
			limitCounter.With(prometheus.Labels{
				poolLabel: pool,
				typeLabel: "time",
			}).Inc()

		case <-ctx.Done():
			t.isMonitoring.Store(false)
			return
		}
	}(t.timeout, ch, t.workerName, t.workerPool)
}

func (t *TimeLimiter) IsMonitoring() bool {
	return t.isMonitoring.Load()
}
