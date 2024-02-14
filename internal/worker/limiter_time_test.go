package worker

import (
	"context"
	"testing"
	"time"
)

func TestLimiterTime(t *testing.T) {
	t.Run("Should add jitter on timeout", func(t *testing.T) {
		lt, ok := newTimeLimiter("name", "pool", time.Millisecond*300, 0.5).(*TimeLimiter)

		if !ok {
			t.Errorf("test %s =  Could not cast to Timelimiter", t.Name())
		}

		upperBound := time.Millisecond * 450
		lowerBound := time.Millisecond * 300
		got := lt.timeout
		if got > upperBound {
			t.Errorf("test %s =  got event %d, want less than %d", t.Name(), got, upperBound)
		}

		if got < lowerBound {
			t.Errorf("test %s =  got event %d, want greater than %d", t.Name(), got, lowerBound)
		}
	})

	t.Run("Should push events on duration timeout", func(t *testing.T) {
		ctx := context.Background()
		lt := newTimeLimiter("name", "pool", time.Millisecond*300, 0)
		ch := make(chan SupervisorEvent)
		if got, want := lt.IsMonitoring(), false; got != want {
			t.Errorf("IsMonitoring test %s  got %t, want %t", t.Name(), got, want)
		}

		lt.Monitor(ch, ctx, nil)
		if got, want := lt.IsMonitoring(), true; got != want {
			t.Errorf("IsMonitoring test %s  got %t, want %t", t.Name(), got, want)
		}

		status := <-ch
		if got, want := status.EventType, ProcessExceededLimit; got != want {
			t.Errorf("validate EventType test %s =  got event %d, want %d", t.Name(), got, want)
		}
		if got, want := status.EventReason, TimeLimitExceeded; got != want {
			t.Errorf("validate EventReason test %s =  got event %d, want %d", t.Name(), got, want)
		}
	})

	t.Run("Should not push events on duration timeout if process terminated", func(t *testing.T) {
		ctx := context.Background()
		ctx, cancel := context.WithCancel(ctx)

		lt := newTimeLimiter("name", "pool", time.Millisecond*300, 0)
		ch := make(chan SupervisorEvent)
		if got, want := lt.IsMonitoring(), false; got != want {
			t.Errorf("IsMonitoring test %s  got %t, want %t", t.Name(), got, want)
		}

		lt.Monitor(ch, ctx, nil)
		if got, want := lt.IsMonitoring(), true; got != want {
			t.Errorf("IsMonitoring test %s  got %t, want %t", t.Name(), got, want)
		}

		cancel()
		for ok := true; ok; ok = lt.IsMonitoring() {
			select {
			case _, ok := <-ch:
				if ok {
					t.Errorf("Test %s got event even after the process is terminated", t.Name())
				}
			default:
			}
		}
	})
}
