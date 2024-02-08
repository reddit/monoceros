package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
)

func startAdminServer(t *testing.T, ctx context.Context, errs chan error, timeout time.Duration) {
	admin := NewAdminInterface(zaptest.NewLogger(t))
	go func() {
		errs <- admin.Run(ctx, AdminConfig{
			// This could cause flakes if the port is retaken
			Port: 9001,
		}, http.NotFoundHandler())
	}()

	serverStarted := false
	for start := time.Now(); time.Since(start) < timeout; {
		resp, err := http.Get("http://0.0.0.0:9001/metrics")
		if err == nil && resp.StatusCode == 200 {
			defer resp.Body.Close()
			serverStarted = true
			break
		}
	}

	if !serverStarted {
		t.Fatalf("%s: admin endpoint could not serve metrics", t.Name())
	}
}

func TestAdminServesProm(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	errs := make(chan error, 1)
	startAdminServer(t, ctx, errs, 15*time.Second)
	cancel()
	err := <-errs
	if err != nil {
		t.Fatalf("%s: unexpected error returned from admin.Run: %t", t.Name(), err)
	}
}

func TestAdminFailsOnDuplicatePort(t *testing.T) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	errs := make(chan error, 1)
	startAdminServer(t, ctx, errs, 15*time.Second)
	err2 := NewAdminInterface(zaptest.NewLogger(t)).Run(ctx, AdminConfig{
		// This could cause flakes if the port is retaken
		Port: 9001,
	}, http.NotFoundHandler())

	if err2 == nil {
		t.Fatalf("%s: admin.Run should fail because of port collision", t.Name())
	}

	cancel()
	err := <-errs
	if err != nil {
		t.Fatalf("%s: unexpected error returned from admin.Run: %t", t.Name(), err)
	}
}

func TestHealthHandlerFunc(t *testing.T) {
	tests := []struct {
		name         string
		isHealthy    bool
		expectStatus int
	}{
		{
			name:         "not healthy returns 503",
			isHealthy:    false,
			expectStatus: http.StatusServiceUnavailable,
		},
		{
			name:         "healthy returns 200",
			isHealthy:    true,
			expectStatus: http.StatusOK,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := HealthHandlerFunc(func() bool {
				return test.isHealthy
			})

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, nil)
			if w.Code != test.expectStatus {
				t.Errorf("status code %d did not match the expected %d", w.Code, test.expectStatus)
			}
		})
	}
}
